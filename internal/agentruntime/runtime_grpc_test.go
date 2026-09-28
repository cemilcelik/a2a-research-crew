package agentruntime

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2agrpc/v1"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"a2a-research-crew/internal/llm"
	"a2a-research-crew/internal/mcpx"
	"a2a-research-crew/internal/memory"
	"a2a-research-crew/internal/toolbox"
)

// TestMarketScoutOverGRPC, gerçek bir gRPC round-trip ile bir ajanı uçtan uca
// çalıştırır: MCP aracı keşfi/çağrısı, LLM tool-calling döngüsü ve A2A artefakt
// üretimi.
func TestMarketScoutOverGRPC(t *testing.T) {
	ctx := context.Background()

	mcpClient, cleanup, err := mcpx.NewInProcessServer(ctx, "web-search", []mcpx.InProcessTool{
		{
			Name:        "web_search",
			Description: "Searches the web",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"query": map[string]any{"type": "string"}},
			},
			Handler: func(_ context.Context, args map[string]any) (string, error) {
				return "Acme market is worth 5 billion USD", nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewInProcessServer() error: %v", err)
	}
	defer cleanup()

	tb, err := toolbox.New(ctx, mcpClient)
	if err != nil {
		t.Fatalf("toolbox.New() error: %v", err)
	}

	provider := llm.NewScripted(
		llm.Response{
			FinishReason: "tool_calls",
			ToolCalls:    []llm.ToolCall{{ID: "call-1", Name: "web_search", Arguments: []byte(`{"query":"acme"}`)}},
		},
		llm.Response{Content: "Market brief: the Acme market is worth 5B USD.", FinishReason: "stop"},
	)

	runtime := New(Spec{
		Name:              "market-scout",
		SystemInstruction: "You are a market research assistant.",
		ArtifactName:      "market-brief",
	}, Deps{
		LLM:    provider,
		Memory: memory.NewMockStore(nil),
		Tools:  tb,
		Logger: discardLogger(),
	})

	addr := startGRPCServer(t, runtime)

	card := &a2a.AgentCard{
		Name: "market-scout",
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(addr, a2a.TransportProtocolGRPC),
		},
	}
	client, err := a2aclient.NewFromCard(ctx, card,
		a2agrpc.WithGRPCTransport(grpc.WithTransportCredentials(insecure.NewCredentials())))
	if err != nil {
		t.Fatalf("NewFromCard() error: %v", err)
	}

	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("research the acme market"))
	result, err := client.SendMessage(ctx, &a2a.SendMessageRequest{Message: msg})
	if err != nil {
		t.Fatalf("SendMessage() error: %v", err)
	}

	task, ok := result.(*a2a.Task)
	if !ok {
		t.Fatalf("result type = %T, want *a2a.Task", result)
	}
	if task.Status.State != a2a.TaskStateCompleted {
		t.Fatalf("state = %s, want completed", task.Status.State)
	}
	if len(task.Artifacts) == 0 || !strings.Contains(task.Artifacts[0].Parts[0].Text(), "5B USD") {
		t.Fatalf("artifacts = %+v, want the market brief", task.Artifacts)
	}
}

func startGRPCServer(t *testing.T, executor a2asrv.AgentExecutor) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen error: %v", err)
	}
	server := grpc.NewServer()
	a2agrpc.NewHandler(a2asrv.NewHandler(executor)).RegisterWith(server)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	return listener.Addr().String()
}
