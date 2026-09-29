package memorymcp

import (
	"context"
	"strings"
	"testing"

	"a2a-research-crew/internal/memory"
)

func TestMemoryMCPRememberAndRecall(t *testing.T) {
	ctx := context.Background()
	store := memory.NewMockStore(nil)
	client, cleanup, err := New(ctx, store, "notes")
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer cleanup()

	remembered, err := client.CallTool(ctx, ToolRemember, map[string]any{
		"content": "the CRM market is highly competitive",
	})
	if err != nil || remembered.IsError {
		t.Fatalf("remember = %+v, err = %v", remembered, err)
	}

	recalled, err := client.CallTool(ctx, ToolRecall, map[string]any{
		"query": "the CRM market is highly competitive",
	})
	if err != nil || recalled.IsError {
		t.Fatalf("recall = %+v, err = %v", recalled, err)
	}
	if !strings.Contains(recalled.Text, "CRM market") {
		t.Fatalf("recall = %q, want the stored note", recalled.Text)
	}
}

func TestMemoryMCPRequiresContent(t *testing.T) {
	ctx := context.Background()
	client, cleanup, err := New(ctx, memory.NewMockStore(nil), "notes")
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer cleanup()

	res, err := client.CallTool(ctx, ToolRemember, map[string]any{})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if !res.IsError {
		t.Fatal("remember with empty content should be a tool error")
	}
}
