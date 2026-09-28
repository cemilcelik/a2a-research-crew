package sqlitemcp

import (
	"context"
	"io"
	"log/slog"
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

// TestCompetitorAnalystToolLoop, agent çalışma döngüsünün gerçek sqlite MCP
// araçlarını kullandığını uçtan uca doğrular.
func TestCompetitorAnalystToolLoop(t *testing.T) {
	ctx := context.Background()
	client, cleanup, err := New(ctx, &config.Config{MCP: config.MCPConfig{SQLiteDir: t.TempDir()}})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer cleanup()

	tb, err := toolbox.New(ctx, client)
	if err != nil {
		t.Fatalf("toolbox.New() error: %v", err)
	}

	provider := llm.NewScripted(
		toolCall("c1", ToolRunSQL, `{"sql":"CREATE TABLE competitors (name TEXT, share REAL)"}`),
		toolCall("c2", ToolRunSQL, `{"sql":"INSERT INTO competitors VALUES ('Alpha', 0.4)"}`),
		toolCall("c3", ToolRunSQL, `{"sql":"SELECT name, share FROM competitors"}`),
		llm.Response{Content: "Competitor matrix: Alpha leads with 0.4 share.", FinishReason: "stop"},
	)

	rt := agentruntime.New(agentruntime.Spec{
		Name:         "competitor-analyst",
		ArtifactName: "competitor-matrix",
	}, agentruntime.Deps{
		LLM:    provider,
		Memory: memory.NewMockStore(nil),
		Tools:  tb,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	execCtx := &a2asrv.ExecutorContext{
		Message:   a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("compare ev makers")),
		TaskID:    a2a.NewTaskID(),
		ContextID: a2a.NewContextID(),
	}

	task := runTask(rt, execCtx)
	if task.Status.State != a2a.TaskStateCompleted {
		t.Fatalf("state = %s, want completed", task.Status.State)
	}
	if len(task.Artifacts) != 1 || !strings.Contains(task.Artifacts[0].Parts[0].Text(), "Competitor matrix") {
		t.Fatalf("artifacts = %+v, want the competitor matrix", task.Artifacts)
	}
}

func toolCall(id, name, args string) llm.Response {
	return llm.Response{
		FinishReason: "tool_calls",
		ToolCalls:    []llm.ToolCall{{ID: id, Name: name, Arguments: []byte(args)}},
	}
}

func runTask(rt *agentruntime.Runtime, execCtx *a2asrv.ExecutorContext) *a2a.Task {
	task := &a2a.Task{}
	for event, err := range rt.Execute(context.Background(), execCtx) {
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
	return task
}
