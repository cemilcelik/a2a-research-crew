// Package memorymcp, bir SemanticStore'u MCP araçları olarak sunar. Orchestrator
// bu sunucu üzerinden geçmiş bilgiyi açıkça hatırlayıp kaydedebilir; böylece
// ajanın "MCP kullanımı" gereksinimi gerçek bir araç yüzeyiyle karşılanır.
package memorymcp

import (
	"context"
	"fmt"
	"strings"

	"a2a-research-crew/internal/mcpx"
	"a2a-research-crew/internal/memory"
)

// Araç adları.
const (
	ToolRecall   = "recall_memory"
	ToolRemember = "remember_note"
)

// New, verilen anlamsal hafıza deposu ve ad alanı için bir MCP istemcisi döner.
func New(ctx context.Context, store memory.SemanticStore, namespace string) (*mcpx.Client, func(), error) {
	return mcpx.NewInProcessServer(ctx, "memory", []mcpx.InProcessTool{
		{
			Name:        ToolRecall,
			Description: "Searches long-term memory for relevant notes and returns the best matches.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "what to search for"},
				},
				"required": []any{"query"},
			},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				return recall(ctx, store, namespace, args)
			},
		},
		{
			Name:        ToolRemember,
			Description: "Stores a note in long-term memory for later recall.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content": map[string]any{"type": "string", "description": "the note to remember"},
				},
				"required": []any{"content"},
			},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				return remember(ctx, store, namespace, args)
			},
		},
	})
}

func recall(ctx context.Context, store memory.SemanticStore, namespace string, args map[string]any) (string, error) {
	query, _ := args["query"].(string)
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query is required")
	}
	memories, err := store.Recall(ctx, namespace, query, 5)
	if err != nil {
		return "", fmt.Errorf("recall failed: %w", err)
	}
	if len(memories) == 0 {
		return "no matching memories", nil
	}
	var b strings.Builder
	for _, mem := range memories {
		fmt.Fprintf(&b, "- %s\n", mem.Content)
	}
	return b.String(), nil
}

func remember(ctx context.Context, store memory.SemanticStore, namespace string, args map[string]any) (string, error) {
	content, _ := args["content"].(string)
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("content is required")
	}
	id, err := store.Remember(ctx, namespace, content, nil)
	if err != nil {
		return "", fmt.Errorf("remember failed: %w", err)
	}
	return fmt.Sprintf("remembered as %s", id), nil
}
