package sqlitemcp

import (
	"context"
	"strings"
	"testing"

	"a2a-research-crew/internal/config"
)

func TestSqliteMCPRunsQueries(t *testing.T) {
	ctx := context.Background()
	client, cleanup, err := New(ctx, &config.Config{MCP: config.MCPConfig{SQLiteDir: t.TempDir()}})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer cleanup()

	if _, err := client.CallTool(ctx, ToolRunSQL, map[string]any{
		"sql": "CREATE TABLE competitors (name TEXT, market_share REAL)",
	}); err != nil {
		t.Fatalf("create table error: %v", err)
	}
	if _, err := client.CallTool(ctx, ToolRunSQL, map[string]any{
		"sql": "INSERT INTO competitors (name, market_share) VALUES ('Alpha', 0.4), ('Beta', 0.3)",
	}); err != nil {
		t.Fatalf("insert error: %v", err)
	}

	res, err := client.CallTool(ctx, ToolRunSQL, map[string]any{
		"sql": "SELECT name, market_share FROM competitors ORDER BY market_share DESC",
	})
	if err != nil {
		t.Fatalf("select error: %v", err)
	}
	if res.IsError {
		t.Fatalf("select returned tool error: %s", res.Text)
	}
	if !strings.Contains(res.Text, "Alpha") || !strings.Contains(res.Text, `"count":2`) {
		t.Fatalf("select result = %s, want Alpha and count 2", res.Text)
	}

	tables, err := client.CallTool(ctx, ToolListTables, nil)
	if err != nil {
		t.Fatalf("list_tables error: %v", err)
	}
	if !strings.Contains(tables.Text, "competitors") {
		t.Fatalf("list_tables = %s, want competitors", tables.Text)
	}
}

func TestSqliteMCPRejectsEmptySQL(t *testing.T) {
	ctx := context.Background()
	client, cleanup, err := New(ctx, &config.Config{MCP: config.MCPConfig{SQLiteDir: t.TempDir()}})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer cleanup()

	res, err := client.CallTool(ctx, ToolRunSQL, map[string]any{"sql": "   "})
	if err != nil {
		t.Fatalf("CallTool transport error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("empty sql = %+v, want tool error", res)
	}
}
