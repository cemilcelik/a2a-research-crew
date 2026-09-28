// Package websearch, market-scout ajanının kullandığı web-arama MCP istemcisini
// üretir. Gerçek bir MCP sunucu URL'si verilirse ona bağlanır; aksi halde
// deterministik, in-process sahte bir arama sunucusu kullanılır.
package websearch

import (
	"context"
	"fmt"
	"strings"

	"a2a-research-crew/internal/config"
	"a2a-research-crew/internal/mcpx"
)

// ToolName, web-arama aracının MCP adıdır.
const ToolName = "web_search"

// New, web-arama MCP istemcisi ve bir kapatma fonksiyonu döner.
func New(ctx context.Context, cfg *config.Config) (*mcpx.Client, func(), error) {
	if cfg.MCP.WebSearchURL != "" {
		headers := map[string]string{}
		if cfg.MCP.WebSearchAPIKey != "" {
			headers["Authorization"] = "Bearer " + cfg.MCP.WebSearchAPIKey
		}
		client, err := mcpx.Connect(ctx, mcpx.ServerConfig{
			Name:    "web-search",
			URL:     cfg.MCP.WebSearchURL,
			Headers: headers,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("websearch: connect: %w", err)
		}
		return client, func() { _ = client.Close() }, nil
	}

	client, cleanup, err := mcpx.NewFakeServer(ctx, "web-search", []mcpx.FakeTool{
		{
			Name:        ToolName,
			Description: "Searches the web for a query and returns summarized results.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "the search query"},
				},
				"required": []any{"query"},
			},
			Handler: fakeSearch,
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("websearch: start fake server: %w", err)
	}
	return client, cleanup, nil
}

func fakeSearch(_ context.Context, args map[string]any) (string, error) {
	query, _ := args["query"].(string)
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query is required")
	}
	return fmt.Sprintf(
		"Mock web results for %q: the market is growing at roughly 12%% CAGR; "+
			"leading players are Alpha, Beta and Gamma; pricing pressure is increasing.",
		query,
	), nil
}
