package memory

import (
	"context"
	"os"
	"testing"

	"a2a-research-crew/internal/llm"
)

func TestMockMessageStore(t *testing.T) {
	store := NewMockStore(nil)
	ctx := context.Background()

	if err := store.Append(ctx, "c1",
		llm.Message{Role: llm.RoleUser, Content: "one"},
		llm.Message{Role: llm.RoleAssistant, Content: "two"},
		llm.Message{Role: llm.RoleUser, Content: "three"},
	); err != nil {
		t.Fatalf("Append() error: %v", err)
	}

	recent, err := store.Recent(ctx, "c1", 2)
	if err != nil {
		t.Fatalf("Recent() error: %v", err)
	}
	if len(recent) != 2 || recent[0].Content != "two" || recent[1].Content != "three" {
		t.Fatalf("Recent() = %+v, want [two three]", recent)
	}
}

func TestMockSemanticStore(t *testing.T) {
	store := NewMockStore(nil)
	ctx := context.Background()

	if _, err := store.Remember(ctx, "user-1", "market size is growing", nil); err != nil {
		t.Fatalf("Remember() error: %v", err)
	}
	if _, err := store.Remember(ctx, "user-1", "competitor pricing is high", nil); err != nil {
		t.Fatalf("Remember() error: %v", err)
	}

	recalled, err := store.Recall(ctx, "user-1", "market size is growing", 1)
	if err != nil {
		t.Fatalf("Recall() error: %v", err)
	}
	if len(recalled) != 1 || recalled[0].Content != "market size is growing" {
		t.Fatalf("Recall() = %+v, want the exact match first", recalled)
	}
	if recalled[0].Score < 0.99 {
		t.Errorf("Score = %v, want ~1 for identical content", recalled[0].Score)
	}
}

func TestMockSemanticIsolatesNamespaces(t *testing.T) {
	store := NewMockStore(nil)
	ctx := context.Background()
	if _, err := store.Remember(ctx, "user-1", "secret alpha", nil); err != nil {
		t.Fatalf("Remember() error: %v", err)
	}

	recalled, err := store.Recall(ctx, "user-2", "secret alpha", 5)
	if err != nil {
		t.Fatalf("Recall() error: %v", err)
	}
	if len(recalled) != 0 {
		t.Fatalf("Recall() across namespaces = %+v, want empty", recalled)
	}
}

func newTestPostgresStore(t *testing.T) *PostgresStore {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set; skipping PostgreSQL integration test")
	}
	store, err := NewPostgresStore(context.Background(), dsn, llm.NewMockEmbedder(64))
	if err != nil {
		t.Fatalf("NewPostgresStore() error: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() error: %v", err)
	}
	if _, err := store.pool.Exec(context.Background(), "TRUNCATE memory_messages, memory_semantic"); err != nil {
		t.Fatalf("truncate error: %v", err)
	}
	t.Cleanup(store.Close)
	return store
}

func TestPostgresMessageStore(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()

	if err := store.Append(ctx, "c1",
		llm.Message{Role: llm.RoleUser, Content: "hello"},
		llm.Message{
			Role:    llm.RoleAssistant,
			Content: "calling tool",
			ToolCalls: []llm.ToolCall{
				{ID: "call-1", Name: "search", Arguments: []byte(`{"q":"x"}`)},
			},
		},
		llm.Message{Role: llm.RoleTool, Content: "result", ToolCallID: "call-1", Name: "search"},
	); err != nil {
		t.Fatalf("Append() error: %v", err)
	}

	recent, err := store.Recent(ctx, "c1", 10)
	if err != nil {
		t.Fatalf("Recent() error: %v", err)
	}
	if len(recent) != 3 {
		t.Fatalf("len(Recent()) = %d, want 3", len(recent))
	}
	if len(recent[1].ToolCalls) != 1 || recent[1].ToolCalls[0].Name != "search" {
		t.Errorf("tool calls not round-tripped: %+v", recent[1].ToolCalls)
	}
	if recent[2].ToolCallID != "call-1" {
		t.Errorf("ToolCallID = %q, want call-1", recent[2].ToolCallID)
	}
}

func TestPostgresSemanticStore(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()

	if _, err := store.Remember(ctx, "user-1", "the market is expanding rapidly", map[string]any{"topic": "market"}); err != nil {
		t.Fatalf("Remember() error: %v", err)
	}
	if _, err := store.Remember(ctx, "user-1", "our competitor lowered prices", nil); err != nil {
		t.Fatalf("Remember() error: %v", err)
	}

	recalled, err := store.Recall(ctx, "user-1", "the market is expanding rapidly", 2)
	if err != nil {
		t.Fatalf("Recall() error: %v", err)
	}
	if len(recalled) != 2 {
		t.Fatalf("len(Recall()) = %d, want 2", len(recalled))
	}
	if recalled[0].Content != "the market is expanding rapidly" {
		t.Errorf("top result = %q, want the exact match", recalled[0].Content)
	}
	if recalled[0].Metadata["topic"] != "market" {
		t.Errorf("metadata not round-tripped: %+v", recalled[0].Metadata)
	}
}
