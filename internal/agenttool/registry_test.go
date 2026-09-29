package agenttool

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2agrpc/v1"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"google.golang.org/grpc"

	"a2a-research-crew/internal/agentruntime"
	"a2a-research-crew/internal/llm"
	"a2a-research-crew/internal/memory"
)

// TestRegistryCallsRemoteAgent, bir uzman ajanı HTTP AgentCard + gRPC üzerinden
// keşfedip araç olarak çağırır ve artefakt metnini döndürür.
func TestRegistryCallsRemoteAgent(t *testing.T) {
	ctx := context.Background()

	provider := llm.NewScripted(llm.Response{Content: "market brief from specialist", FinishReason: "stop"})
	rt := agentruntime.New(agentruntime.Spec{Name: "market-scout", ArtifactName: "market-brief"}, agentruntime.Deps{
		LLM:    provider,
		Memory: memory.NewMockStore(nil),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	cardURL := startSpecialist(t, rt)

	registry := New([]Agent{{
		Name:        "market_scout",
		URL:         cardURL,
		Description: "market research",
	}}, nil)
	defer func() { _ = registry.Close() }()

	res, err := registry.CallTool(ctx, "market_scout", map[string]any{"query": "ev market"})
	if err != nil {
		t.Fatalf("CallTool() error: %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool() IsError = true: %s", res.Text)
	}
	if !strings.Contains(res.Text, "market brief from specialist") {
		t.Fatalf("CallTool() = %q, want the specialist artifact", res.Text)
	}
}

func startSpecialist(t *testing.T, executor a2asrv.AgentExecutor) string {
	t.Helper()

	grpcListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grpc listen error: %v", err)
	}
	grpcServer := grpc.NewServer()
	a2agrpc.NewHandler(a2asrv.NewHandler(executor)).RegisterWith(grpcServer)
	go func() { _ = grpcServer.Serve(grpcListener) }()

	card := &a2a.AgentCard{
		Name: "market-scout",
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(grpcListener.Addr().String(), a2a.TransportProtocolGRPC),
		},
	}
	cardMux := http.NewServeMux()
	cardMux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(card))
	cardListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("card listen error: %v", err)
	}
	cardServer := &http.Server{Handler: cardMux}
	go func() { _ = cardServer.Serve(cardListener) }()

	t.Cleanup(func() {
		grpcServer.Stop()
		_ = cardServer.Close()
	})
	return "http://" + cardListener.Addr().String()
}
