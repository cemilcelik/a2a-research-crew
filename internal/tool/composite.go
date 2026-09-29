package tool

import (
	"context"
	"fmt"

	"a2a-research-crew/internal/llm"
)

// Composite, birden çok Provider'ı tek bir Provider gibi sunar. Araç adları
// sağlayıcılar arasında benzersiz olmalıdır.
type Composite struct {
	providers []Provider
	defs      []llm.ToolDef
	owners    map[string]Provider
}

var _ Provider = (*Composite)(nil)

// NewComposite, verilen sağlayıcıları birleştirir. Aynı ada sahip iki araç
// varsa hata döner.
func NewComposite(providers ...Provider) (*Composite, error) {
	composite := &Composite{owners: make(map[string]Provider)}
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		composite.providers = append(composite.providers, provider)
		for _, def := range provider.ToolDefs() {
			if _, dup := composite.owners[def.Name]; dup {
				return nil, fmt.Errorf("tool: duplicate tool %q across providers", def.Name)
			}
			composite.owners[def.Name] = provider
			composite.defs = append(composite.defs, def)
		}
	}
	return composite, nil
}

// ToolDefs implements Provider.
func (c *Composite) ToolDefs() []llm.ToolDef { return c.defs }

// CallTool implements Provider.
func (c *Composite) CallTool(ctx context.Context, name string, args map[string]any) (Result, error) {
	provider, ok := c.owners[name]
	if !ok {
		return Result{}, fmt.Errorf("tool: unknown tool %q", name)
	}
	return provider.CallTool(ctx, name, args)
}
