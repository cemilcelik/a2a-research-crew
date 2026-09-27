package mcpx

import (
	"context"
	"encoding/json"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"a2a-research-crew/internal/buildinfo"
)

// FakeTool, in-process (sahte) bir MCP sunucusunda sunulacak tek bir araçtır.
// Gerçek API anahtarı olmadan dry-run yapmayı ve testleri mümkün kılar.
type FakeTool struct {
	Name        string
	Description string
	// Schema, aracın argümanları için JSON Schema'dır. Boşsa basit bir nesne
	// şeması kullanılır.
	Schema map[string]any
	// Handler, araç çağrısını işler.
	Handler func(ctx context.Context, args map[string]any) (string, error)
}

// NewFakeServer, verilen araçlarla in-process bir MCP sunucusu başlatır ve ona
// bağlı bir Client döner. Dönen kapatma fonksiyonu oturumu sonlandırır.
func NewFakeServer(ctx context.Context, name string, tools []FakeTool) (*Client, func(), error) {
	server := sdk.NewServer(&sdk.Implementation{
		Name:    name,
		Version: buildinfo.Version,
	}, nil)

	for _, tool := range tools {
		schema := any(map[string]any{"type": "object", "properties": map[string]any{}})
		if tool.Schema != nil {
			schema = tool.Schema
		}
		server.AddTool(
			&sdk.Tool{Name: tool.Name, Description: tool.Description, InputSchema: schema},
			newFakeHandler(tool),
		)
	}

	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("mcpx: start fake server %q: %w", name, err)
	}

	client, err := ConnectTransport(ctx, name, clientTransport)
	if err != nil {
		_ = serverSession.Close()
		return nil, nil, err
	}

	cleanup := func() {
		_ = client.Close()
		_ = serverSession.Close()
	}
	return client, cleanup, nil
}

func newFakeHandler(tool FakeTool) sdk.ToolHandler {
	return func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		args := map[string]any{}
		if raw := req.Params.Arguments; len(raw) > 0 {
			if err := json.Unmarshal(raw, &args); err != nil {
				return errorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}
		}
		text, err := tool.Handler(ctx, args)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		return &sdk.CallToolResult{
			Content: []sdk.Content{&sdk.TextContent{Text: text}},
		}, nil
	}
}

func errorResult(message string) *sdk.CallToolResult {
	return &sdk.CallToolResult{
		IsError: true,
		Content: []sdk.Content{&sdk.TextContent{Text: message}},
	}
}
