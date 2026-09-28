// Package main, rakip analizi uzmanı ajanını (competitor-analyst) başlatır.
// Ajan, izole bir sqlite çalışma alanında SQL çalıştırarak rakip verisini
// analiz eder ve bir "competitor-matrix" artefaktı üretir.
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
	"a2a-research-crew/internal/sqlitemcp"
	"a2a-research-crew/internal/toolbox"
)

const (
	agentName       = "competitor-analyst"
	defaultGRPCPort = 9102
	defaultCardPort = 9202
)

const systemInstruction = `You are the competitor analysis specialist of a research crew.
Your job: analyze competitors and produce a comparison matrix.

Rules:
- Use the run_sql tool to create tables, load competitor data, and query it.
- Use list_tables to inspect what data already exists before assuming.
- The SQL database is an isolated workspace; it contains only what you load there.
- Produce a concise "competitor matrix" covering: players, positioning, strengths and weaknesses.
- Base every claim on data you actually loaded or queried. Do not invent numbers.
- Write the result in the language of the request (default: English).`

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
			Model:             os.Getenv("COMPETITOR_ANALYST_MODEL"),
			ArtifactName:      "competitor-matrix",
		},
		Description: "Competitor analyst that compares players using an isolated SQL workspace.",
		Version:     "0.1.0",
		Skill: agent.Skill{
			ID:          "competitor_analysis",
			Name:        "Competitor analysis",
			Description: "Analyzes competitors with SQL and produces a comparison matrix.",
			Tags:        []string{"competitors", "analysis", "sql"},
			Examples:    []string{"Compare the main EV manufacturers", "Build a competitor matrix for CRM vendors"},
		},
		ListenGRPCPort: opts.GRPCPort,
		ListenCardPort: opts.CardPort,
		BuildTools:     buildTools,
	}

	if err := agentapp.Run(context.Background(), cfg, appOpts); err != nil {
		log.Fatalf("competitor-analyst stopped: %v", err)
	}
}

// buildTools, izole sqlite MCP sunucusunu kurar ve toolbox'a sarar.
func buildTools(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*agentapp.Toolset, error) {
	client, cleanup, err := sqlitemcp.New(ctx, cfg)
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
