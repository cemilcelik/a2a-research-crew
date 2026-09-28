package agentruntime

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"a2a-research-crew/internal/llm"
	"a2a-research-crew/internal/memory"
	"a2a-research-crew/internal/tool"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type stubTools struct {
	defs  []llm.ToolDef
	calls []string
}

func (s *stubTools) ToolDefs() []llm.ToolDef { return s.defs }

func (s *stubTools) CallTool(_ context.Context, name string, args map[string]any) (tool.Result, error) {
	s.calls = append(s.calls, name)
	return tool.Result{Text: "search results"}, nil
}

// collect, üretilen olayları tüketir ve döner.
func collect(seq func(yield func(a2a.Event, error) bool)) []a2a.Event {
	var events []a2a.Event
	for event, err := range seq {
		if err != nil {
			return events
		}
		events = append(events, event)
	}
	return events
}

func newExecCtx(text string) *a2asrv.ExecutorContext {
	return &a2asrv.ExecutorContext{
		Message:   a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(text)),
		TaskID:    a2a.NewTaskID(),
		ContextID: a2a.NewContextID(),
	}
}

func TestRuntimeToolCallingProducesArtifact(t *testing.T) {
	provider := llm.NewScripted(
		llm.Response{
			FinishReason: "tool_calls",
			ToolCalls:    []llm.ToolCall{{ID: "call-1", Name: "web_search", Arguments: []byte(`{"q":"acme"}`)}},
		},
		llm.Response{Content: "final market brief", FinishReason: "stop"},
	)
	tools := &stubTools{defs: []llm.ToolDef{{Name: "web_search", Parameters: []byte(`{"type":"object"}`)}}}
	store := memory.NewMockStore(nil)

	rt := New(Spec{
		Name:              "market-scout",
		SystemInstruction: "You research markets.",
		ArtifactName:      "market-brief",
	}, Deps{LLM: provider, Memory: store, Tools: tools, Logger: discardLogger()})

	execCtx := newExecCtx("research acme corp")
	events := collect(rt.Execute(context.Background(), execCtx))

	if len(tools.calls) != 1 || tools.calls[0] != "web_search" {
		t.Fatalf("tool calls = %v, want [web_search]", tools.calls)
	}

	task := lastTask(events)
	if task == nil {
		t.Fatal("no task event produced")
	}
	if task.Status.State != a2a.TaskStateCompleted {
		t.Fatalf("state = %s, want completed", task.Status.State)
	}
	if len(task.Artifacts) != 1 || !strings.Contains(task.Artifacts[0].Parts[0].Text(), "final market brief") {
		t.Fatalf("artifacts = %+v, want final market brief", task.Artifacts)
	}
	if task.Artifacts[0].Name != "market-brief" {
		t.Errorf("artifact name = %q, want market-brief", task.Artifacts[0].Name)
	}
}

func TestRuntimeNoTools(t *testing.T) {
	provider := llm.NewScripted(llm.Response{Content: "hello from llm", FinishReason: "stop"})
	rt := New(Spec{Name: "simple", SystemInstruction: "be helpful"}, Deps{
		LLM:    provider,
		Memory: memory.NewMockStore(nil),
		Logger: discardLogger(),
	})

	events := collect(rt.Execute(context.Background(), newExecCtx("hi")))
	task := lastTask(events)
	if task == nil || task.Status.State != a2a.TaskStateCompleted {
		t.Fatalf("task = %+v, want completed", task)
	}
}

func TestRuntimeMaxIterations(t *testing.T) {
	// LLM sürekli araç çağırırsa döngü sınırında hata vermeli.
	provider := llm.NewMock("m")
	provider.CompleteFunc = func(context.Context, llm.Request) (llm.Response, error) {
		return llm.Response{
			FinishReason: "tool_calls",
			ToolCalls:    []llm.ToolCall{{ID: "c", Name: "loop", Arguments: []byte(`{}`)}},
		}, nil
	}
	rt := New(Spec{Name: "looper", MaxToolIterations: 2}, Deps{
		LLM:    provider,
		Memory: memory.NewMockStore(nil),
		Tools:  &stubTools{},
		Logger: discardLogger(),
	})

	events := collect(rt.Execute(context.Background(), newExecCtx("go")))
	task := lastTask(events)
	if task == nil || task.Status.State != a2a.TaskStateFailed {
		t.Fatalf("task state = %v, want failed after max iterations", task)
	}
}

// lastTask, olay akışındaki son görev durumunu döner.
func lastTask(events []a2a.Event) *a2a.Task {
	task := &a2a.Task{}
	found := false
	for _, event := range events {
		switch e := event.(type) {
		case *a2a.Task:
			task = e
			found = true
		case *a2a.TaskStatusUpdateEvent:
			task.Status.State = e.Status.State
			found = true
		case *a2a.TaskArtifactUpdateEvent:
			task.Artifacts = append(task.Artifacts, e.Artifact)
		}
	}
	if !found {
		return nil
	}
	return task
}
