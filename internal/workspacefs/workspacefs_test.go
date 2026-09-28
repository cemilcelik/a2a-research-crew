package workspacefs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"a2a-research-crew/internal/config"
)

func TestWorkspaceWriteReadList(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	client, cleanup, err := New(ctx, &config.Config{MCP: config.MCPConfig{FSRoot: root}})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer cleanup()

	write, err := client.CallTool(ctx, ToolWriteFile, map[string]any{
		"path":    "reports/final.md",
		"content": "# Final report",
	})
	if err != nil || write.IsError {
		t.Fatalf("write_file = %+v, err = %v", write, err)
	}

	read, err := client.CallTool(ctx, ToolReadFile, map[string]any{"path": "reports/final.md"})
	if err != nil || read.IsError {
		t.Fatalf("read_file = %+v, err = %v", read, err)
	}
	if read.Text != "# Final report" {
		t.Fatalf("read_file = %q, want '# Final report'", read.Text)
	}

	list, err := client.CallTool(ctx, ToolListDir, map[string]any{"path": "reports"})
	if err != nil || list.IsError {
		t.Fatalf("list_dir = %+v, err = %v", list, err)
	}
	if !strings.Contains(list.Text, "final.md") {
		t.Fatalf("list_dir = %q, want final.md", list.Text)
	}
}

func TestWorkspaceContainsPathTraversal(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	client, cleanup, err := New(ctx, &config.Config{MCP: config.MCPConfig{FSRoot: root}})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer cleanup()

	// "../../escape.txt" yolu kök dışına çıkmamalı; kök içine hapsedilmeli.
	if _, err := client.CallTool(ctx, ToolWriteFile, map[string]any{
		"path":    "../../escape.txt",
		"content": "escaped",
	}); err != nil {
		t.Fatalf("write_file transport error: %v", err)
	}

	parent := filepath.Dir(root)
	if _, err := os.Stat(filepath.Join(parent, "escape.txt")); err == nil {
		t.Fatal("path traversal escaped the workspace root")
	}
	// İçerik kök içinde (escape.txt olarak) bulunmalı.
	if _, err := os.Stat(filepath.Join(root, "escape.txt")); err != nil {
		t.Fatalf("expected contained file under root: %v", err)
	}
}
