package tool

import (
	"context"
	"testing"

	"a2a-research-crew/internal/llm"
)

type stubProvider struct {
	name  string
	tools []string
}

func (s *stubProvider) ToolDefs() []llm.ToolDef {
	defs := make([]llm.ToolDef, 0, len(s.tools))
	for _, name := range s.tools {
		defs = append(defs, llm.ToolDef{Name: name, Description: s.name})
	}
	return defs
}

func (s *stubProvider) CallTool(_ context.Context, name string, _ map[string]any) (Result, error) {
	return Result{Text: s.name + ":" + name}, nil
}

func TestCompositeRoutesCalls(t *testing.T) {
	composite, err := NewComposite(
		&stubProvider{name: "a", tools: []string{"alpha"}},
		&stubProvider{name: "b", tools: []string{"beta"}},
	)
	if err != nil {
		t.Fatalf("NewComposite() error: %v", err)
	}
	if len(composite.ToolDefs()) != 2 {
		t.Fatalf("ToolDefs() = %+v, want 2", composite.ToolDefs())
	}

	res, err := composite.CallTool(context.Background(), "beta", nil)
	if err != nil {
		t.Fatalf("CallTool() error: %v", err)
	}
	if res.Text != "b:beta" {
		t.Fatalf("CallTool() = %q, want b:beta", res.Text)
	}
}

func TestCompositeRejectsDuplicateTools(t *testing.T) {
	if _, err := NewComposite(
		&stubProvider{name: "a", tools: []string{"dup"}},
		&stubProvider{name: "b", tools: []string{"dup"}},
	); err == nil {
		t.Fatal("NewComposite() = nil error, want duplicate error")
	}
}

func TestCompositeUnknownTool(t *testing.T) {
	composite, _ := NewComposite(&stubProvider{name: "a", tools: []string{"alpha"}})
	if _, err := composite.CallTool(context.Background(), "missing", nil); err == nil {
		t.Fatal("CallTool() = nil error, want unknown tool error")
	}
}
