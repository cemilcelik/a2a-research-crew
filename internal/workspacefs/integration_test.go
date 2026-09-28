package workspacefs

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"a2a-research-crew/internal/agentruntime"
	"a2a-research-crew/internal/config"
	"a2a-research-crew/internal/llm"
	"a2a-research-crew/internal/memory"
	"a2a-research-crew/internal/toolbox"
)

// TestReportWriterToolLoop, agent çalışma döngüsünün gerçek dosya sistemi MCP
// aracını kullandığını ve dosyanın çalışma alanına yazıldığını doğrular.
func TestReportWriterToolLoop(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	client, cleanup, err := New(ctx, &config.Config{MCP: config.MCPConfig{FSRoot: root}})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer cleanup()

	tb, err := toolbox.New(ctx, client)
	if err != nil {
		t.Fatalf("toolbox.New() error: %v", err)
	}

	provider := llm.NewScripted(
		llm.Response{
			FinishReason: "tool_calls",
			ToolCalls: []llm.ToolCall{{
				ID:        "c1",
				Name:      ToolWriteFile,
				Arguments: []byte(`{"path":"final-report.md","content":"# Final report"}`),
			}},
		},
		llm.Response{Content: "Final report written.", FinishReason: "stop"},
	)

	rt := agentruntime.New(agentruntime.Spec{
		Name:         "report-writer",
		ArtifactName: "final-report",
	}, agentruntime.Deps{
		LLM:    provider,
		Memory: memory.NewMockStore(nil),
		Tools:  tb,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	execCtx := &a2asrv.ExecutorContext{
		Message:   a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("write the report")),
		TaskID:    a2a.NewTaskID(),
		ContextID: a2a.NewContextID(),
	}

	task := &a2a.Task{}
	for event, err := range rt.Execute(ctx, execCtx) {
		if err != nil {
			break
		}
		switch e := event.(type) {
		case *a2a.Task:
			task = e
		case *a2a.TaskStatusUpdateEvent:
			task.Status.State = e.Status.State
		case *a2a.TaskArtifactUpdateEvent:
			task.Artifacts = append(task.Artifacts, e.Artifact)
		}
	}

	if task.Status.State != a2a.TaskStateCompleted {
		t.Fatalf("state = %s, want completed", task.Status.State)
	}
	data, err := os.ReadFile(filepath.Join(root, "final-report.md"))
	if err != nil {
		t.Fatalf("reported file not written: %v", err)
	}
	if !strings.Contains(string(data), "# Final report") {
		t.Fatalf("file content = %q, want the report", string(data))
	}
}
