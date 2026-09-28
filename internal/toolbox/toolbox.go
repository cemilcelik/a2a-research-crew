// Package toolbox, birden fazla MCP sunucusundaki araçları tek bir arayüzde
// toplar. Ajan çalışma zamanı araç tanımlarını buradan alır ve çağrıları buraya
// yönlendirir.
package toolbox

import (
	"context"
	"fmt"

	"a2a-research-crew/internal/llm"
	"a2a-research-crew/internal/mcpx"
	"a2a-research-crew/internal/tool"
)

// Toolbox, bağlı MCP istemcilerindeki araçları birleştirir.
type Toolbox struct {
	clients []*mcpx.Client
	defs    []llm.ToolDef
	owners  map[string]*mcpx.Client
}

var _ tool.Provider = (*Toolbox)(nil)

// New, verilen MCP istemcilerindeki araçları keşfeder. Araç adları tüm
// sunucular arasında benzersiz olmalıdır.
func New(ctx context.Context, clients ...*mcpx.Client) (*Toolbox, error) {
	tb := &Toolbox{
		clients: clients,
		owners:  make(map[string]*mcpx.Client, 0),
	}
	for _, client := range clients {
		tools, err := client.ListTools(ctx)
		if err != nil {
			return nil, err
		}
		for _, tool := range tools {
			if existing, dup := tb.owners[tool.Name]; dup {
				return nil, fmt.Errorf("toolbox: duplicate tool %q on %q and %q", tool.Name, existing.Name(), client.Name())
			}
			tb.owners[tool.Name] = client
			tb.defs = append(tb.defs, llm.ToolDef{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			})
		}
	}
	return tb, nil
}

// ToolDefs, keşfedilen tüm araç tanımlarını döner.
func (t *Toolbox) ToolDefs() []llm.ToolDef { return t.defs }

// HasTools, en az bir aracın mevcut olup olmadığını bildirir.
func (t *Toolbox) HasTools() bool { return len(t.defs) > 0 }

// CallTool, verilen aracı sahibi MCP sunucusunda çağırır.
func (t *Toolbox) CallTool(ctx context.Context, name string, args map[string]any) (tool.Result, error) {
	client, ok := t.owners[name]
	if !ok {
		return tool.Result{}, fmt.Errorf("toolbox: unknown tool %q", name)
	}
	res, err := client.CallTool(ctx, name, args)
	if err != nil {
		return tool.Result{}, err
	}
	return tool.Result{Text: res.Text, IsError: res.IsError}, nil
}

// Close, tüm MCP istemcilerini kapatır.
func (t *Toolbox) Close() error {
	var firstErr error
	for _, client := range t.clients {
		if err := client.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
