// Package main, sistemi dış dünyaya HTTP+JSON/REST üzerinden açan organizatör
// ajanını (orchestrator) başlatır. Orchestrator, JWT uçlarını yayınlar, bir
// araştırma isteğini planlar, uzman ajanlara gRPC üzerinden devreder ve
// sonuçları tek bir raporda sentezler.
package main

import (
	"context"
	"log"
	"log/slog"
	"os"

	"a2a-research-crew/internal/agent"
	"a2a-research-crew/internal/agentapp"
	"a2a-research-crew/internal/agentruntime"
	"a2a-research-crew/internal/agenttool"
	"a2a-research-crew/internal/config"
	"a2a-research-crew/internal/entrypoint"
	"a2a-research-crew/internal/memory"
	"a2a-research-crew/internal/memorymcp"
	"a2a-research-crew/internal/tool"
	"a2a-research-crew/internal/toolbox"
)

const (
	agentName       = "orchestrator"
	defaultRESTPort = 9100
	defaultCardPort = 9200
)

const systemInstruction = `You are the orchestrator of a research crew.
Your job: plan the research, delegate to specialist agents, and synthesize a final report.

Available specialists (use them as tools):
- market_scout: gathers market size, growth and trends.
- competitor_analyst: analyzes competitors using SQL.
- report_writer: turns findings into a structured final report.

Method:
1. If the request is missing a concrete company/product, call ask_user first.
2. Delegate to market_scout and competitor_analyst to gather evidence.
3. Then call report_writer with the collected findings to produce the final report.
4. Return the final report as your answer.

Use your long-term memory tools to recall relevant prior research when useful.
Write in the language of the request (default: English).`

func main() {
	opts := entrypoint.ParseServer(agentName, entrypoint.ServerDefaults{
		RESTPort: defaultRESTPort,
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
			Model:             os.Getenv("ORCHESTRATOR_MODEL"),
			ArtifactName:      "research-report",
			MaxToolIterations: 8,
			ClarificationTool: "ask_user",
		},
		Description: "Research orchestrator that plans, delegates and synthesizes a final report.",
		Version:     "0.1.0",
		Skill: agent.Skill{
			ID:          "research_orchestration",
			Name:        "Research orchestration",
			Description: "Plans research, delegates to specialists and synthesizes the result.",
			Tags:        []string{"orchestration", "research", "planning"},
			Examples:    []string{"Research the CRM market and produce a report"},
		},
		// Dış istek REST ile karşılanır; orchestrator gRPC açmaz.
		RESTPort:      opts.RESTPort,
		CardPort:      opts.CardPort,
		BuildTools:    buildTools,
		AuthEndpoints: true,
	}

	if err := agentapp.Run(context.Background(), cfg, appOpts); err != nil {
		log.Fatalf("orchestrator stopped: %v", err)
	}
}

// buildTools, uzman ajanları A2A araçları ve hafızayı MCP araçları olarak
// birleştirir.
func buildTools(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*agentapp.Toolset, error) {
	registry := agenttool.New([]agenttool.Agent{
		{
			Name:        "market_scout",
			URL:         cfg.Agents.MarketScoutURL,
			Description: "Market research specialist. Send a query about a market and get a market brief.",
		},
		{
			Name:        "competitor_analyst",
			URL:         cfg.Agents.CompetitorAnalystURL,
			Description: "Competitor analyst. Send a query and get a competitor comparison matrix.",
		},
		{
			Name:        "report_writer",
			URL:         cfg.Agents.ReportWriterURL,
			Description: "Report writer. Send the collected findings and get the final structured report.",
		},
	}, nil)

	store, err := memory.NewPostgresStore(ctx, cfg.Postgres.DSN(), agentapp.NewEmbedder(cfg))
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(ctx); err != nil {
		store.Close()
		return nil, err
	}
	memoryClient, memoryCleanup, err := memorymcp.New(ctx, store, "orchestrator-notes")
	if err != nil {
		store.Close()
		return nil, err
	}
	memoryTools, err := toolbox.New(ctx, memoryClient)
	if err != nil {
		memoryCleanup()
		store.Close()
		return nil, err
	}

	composite, err := tool.NewComposite(registry, memoryTools)
	if err != nil {
		memoryCleanup()
		store.Close()
		return nil, err
	}

	logger.Info("orchestrator tools ready", "agent", agentName, "tools", len(composite.ToolDefs()))
	return &agentapp.Toolset{
		Provider: composite,
		Close: func() error {
			memoryCleanup()
			store.Close()
			return registry.Close()
		},
	}, nil
}
