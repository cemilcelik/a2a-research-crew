package mcpx

import (
	"context"
	"strings"
	"testing"
)

func TestFakeServerListAndCall(t *testing.T) {
	ctx := context.Background()
	client, cleanup, err := NewInProcessServer(ctx, "fake", []InProcessTool{
		{
			Name:        "greet",
			Description: "Greets a person",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"name": map[string]any{"type": "string"}},
			},
			Handler: func(_ context.Context, args map[string]any) (string, error) {
				name, _ := args["name"].(string)
				return "hello " + name, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewInProcessServer() error: %v", err)
	}
	defer cleanup()

	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools() error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "greet" {
		t.Fatalf("ListTools() = %+v, want one tool named greet", tools)
	}

	defs, err := client.ToolDefs(ctx)
	if err != nil {
		t.Fatalf("ToolDefs() error: %v", err)
	}
	if len(defs) != 1 || defs[0].Description != "Greets a person" {
		t.Fatalf("ToolDefs() = %+v, want greet def", defs)
	}

	res, err := client.CallTool(ctx, "greet", map[string]any{"name": "ada"})
	if err != nil {
		t.Fatalf("CallTool() error: %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool() IsError = true, text = %q", res.Text)
	}
	if res.Text != "hello ada" {
		t.Errorf("CallTool() text = %q, want %q", res.Text, "hello ada")
	}
}

func TestFakeServerToolError(t *testing.T) {
	ctx := context.Background()
	client, cleanup, err := NewInProcessServer(ctx, "fake", []InProcessTool{
		{
			Name: "boom",
			Handler: func(_ context.Context, _ map[string]any) (string, error) {
				return "", context.DeadlineExceeded
			},
		},
	})
	if err != nil {
		t.Fatalf("NewInProcessServer() error: %v", err)
	}
	defer cleanup()

	res, err := client.CallTool(ctx, "boom", nil)
	if err != nil {
		t.Fatalf("CallTool() transport error: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Text, "deadline") {
		t.Errorf("CallTool() = %+v, want IsError with deadline message", res)
	}
}
