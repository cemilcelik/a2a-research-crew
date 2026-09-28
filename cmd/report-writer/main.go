// Package main, rapor yazımı uzmanı ajanını (report-writer) başlatır. Ajan,
// bulguları sentezleyip kısıtlanmış bir dosya sistemi çalışma alanına yazar ve
// bir "final-report" artefaktı üretir.
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
	"a2a-research-crew/internal/workspacefs"
)

const (
	agentName       = "report-writer"
	defaultGRPCPort = 9103
	defaultCardPort = 9203
)

const systemInstruction = `You are the report writer of a research crew.
Your job: synthesize the provided findings into a clear, well-structured final report.

Rules:
- Use the write_file tool to save the report into the workspace as "final-report.md".
- You may use list_dir and read_file to check existing files.
- The report must have: title, executive summary, key findings, and recommendations.
- Use only the findings provided in the request; do not invent facts.
- Write the report in the language of the request (default: English).`

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
			Model:             os.Getenv("REPORT_WRITER_MODEL"),
			ArtifactName:      "final-report",
		},
		Description: "Report writer that synthesizes findings into a final report artifact.",
		Version:     "0.1.0",
		Skill: agent.Skill{
			ID:          "report_writing",
			Name:        "Report writing",
			Description: "Synthesizes research findings into a structured final report.",
			Tags:        []string{"report", "writing", "synthesis"},
			Examples:    []string{"Write the final research report", "Turn these findings into an executive summary"},
		},
		ListenGRPCPort: opts.GRPCPort,
		ListenCardPort: opts.CardPort,
		BuildTools:     buildTools,
	}

	if err := agentapp.Run(context.Background(), cfg, appOpts); err != nil {
		log.Fatalf("report-writer stopped: %v", err)
	}
}

// buildTools, kısıtlanmış dosya sistemi MCP sunucusunu kurar ve toolbox'a sarar.
func buildTools(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*agentapp.Toolset, error) {
	client, cleanup, err := workspacefs.New(ctx, cfg)
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
