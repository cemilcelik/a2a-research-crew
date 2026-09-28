package toolbox

import (
	"context"
	"testing"

	"a2a-research-crew/internal/mcpx"
)

func newFakeClient(t *testing.T, name string, toolNames ...string) *mcpx.Client {
	t.Helper()
	tools := make([]mcpx.InProcessTool, 0, len(toolNames))
	for _, toolName := range toolNames {
		tools = append(tools, mcpx.InProcessTool{
			Name: toolName,
			Handler: func(_ context.Context, _ map[string]any) (string, error) {
				return name + ":" + toolName, nil
			},
		})
	}
	client, cleanup, err := mcpx.NewInProcessServer(context.Background(), name, tools)
	if err != nil {
		t.Fatalf("NewInProcessServer(%s) error: %v", name, err)
	}
	t.Cleanup(cleanup)
	return client
}

func TestToolboxAggregatesAndCalls(t *testing.T) {
	ctx := context.Background()
	a := newFakeClient(t, "alpha", "search")
	b := newFakeClient(t, "beta", "fetch")

	tb, err := New(ctx, a, b)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if !tb.HasTools() || len(tb.ToolDefs()) != 2 {
		t.Fatalf("ToolDefs() = %+v, want 2 tools", tb.ToolDefs())
	}

	res, err := tb.CallTool(ctx, "fetch", nil)
	if err != nil {
		t.Fatalf("CallTool() error: %v", err)
	}
	if res.Text != "beta:fetch" {
		t.Errorf("CallTool() = %q, want beta:fetch", res.Text)
	}
}

func TestToolboxRejectsDuplicateToolNames(t *testing.T) {
	ctx := context.Background()
	if _, err := New(ctx,
		newFakeClient(t, "alpha", "search"),
		newFakeClient(t, "beta", "search"),
	); err == nil {
		t.Fatal("New() = nil error, want duplicate tool error")
	}
}

func TestToolboxUnknownTool(t *testing.T) {
	ctx := context.Background()
	tb, err := New(ctx, newFakeClient(t, "alpha", "search"))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if _, err := tb.CallTool(ctx, "missing", nil); err == nil {
		t.Fatal("CallTool() = nil error, want unknown tool error")
	}
}
