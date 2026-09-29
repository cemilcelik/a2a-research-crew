// Package main, pazar araştırması uzmanı ajanını (market-scout) başlatır.
// Ajan, web-arama MCP aracını kullanarak pazar bilgisi toplar ve bir
// "market-brief" artefaktı üretir.
package main

import (
	"context"
	"log"
	"log/slog"
	"os"

	"a2a-research-crew/internal/agent"
	"a2a-research-crew/internal/agentapp"
	"a2a-research-crew/internal/agentruntime"
	"a2a-research-crew/internal/config"
	"a2a-research-crew/internal/entrypoint"
	"a2a-research-crew/internal/toolbox"
	"a2a-research-crew/internal/websearch"
)

const (
	agentName       = "market-scout"
	defaultGRPCPort = 9101
	defaultCardPort = 9201
)

const systemInstruction = `You are the market research specialist of a research crew.
Your job: gather and summarize market information for the requested company or product.

Rules:
- Use the web_search tool to gather current market information before answering.
- Produce a concise "market brief" with: market size and growth, key trends, and main players.
- Cite the evidence you used. Do not invent facts; if data is missing, say so.
- Write the brief in the language of the request (default: English).`

func main() {
	opts := entrypoint.ParseServer(agentName, entrypoint.ServerDefaults{
		GRPCPort: defaultGRPCPort,
		CardPort: defaultCardPort,
	})

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	cfg.LogLevel = opts.LogLevel

	appOpts := agentapp.Options{
		Spec: agentruntime.Spec{
			Name:              agentName,
			SystemInstruction: systemInstruction,
			Model:             os.Getenv("MARKET_SCOUT_MODEL"),
			ArtifactName:      "market-brief",
		},
		Description: "Market research specialist that gathers market size, trends and players.",
		Version:     "0.1.0",
		Skill: agent.Skill{
			ID:          "market_research",
			Name:        "Market research",
			Description: "Gathers and summarizes market data for a company or product.",
			Tags:        []string{"market", "research", "trends"},
			Examples:    []string{"Research the electric vehicle market", "How big is the CRM market?"},
		},
		GRPCPort:   opts.GRPCPort,
		CardPort:   opts.CardPort,
		BuildTools: buildTools,
	}

	if err := agentapp.Run(context.Background(), cfg, appOpts); err != nil {
		log.Fatalf("market-scout stopped: %v", err)
	}
}

// buildTools, web-arama MCP istemcisini kurar ve toolbox'a sarar.
func buildTools(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*agentapp.Toolset, error) {
	client, cleanup, err := websearch.New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	tb, err := toolbox.New(ctx, client)
	if err != nil {
		cleanup()
		return nil, err
	}
	logger.Info("mcp tools ready", "agent", agentName, "tools", len(tb.ToolDefs()))
	return &agentapp.Toolset{
		Provider: tb,
		Close: func() error {
			defer cleanup()
			return tb.Close()
		},
	}, nil
}
