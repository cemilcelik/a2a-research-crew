// Package tool, ajanlar ile araç sağlayıcıları arasındaki ortak sözleşmeyi
// tanımlar. Hem MCP tabanlı toolbox hem de test stub'ları bu arayüzü uygular.
package tool

import (
	"context"

	"a2a-research-crew/internal/llm"
)

// Result, bir araç çağrısının sonucudur.
type Result struct {
	Text    string
	IsError bool
}

// Provider, modele sunulacak araçları ve çağrı yönlendirmesini sağlar.
type Provider interface {
	// ToolDefs, modele sunulacak araç tanımlarını döner.
	ToolDefs() []llm.ToolDef
	// CallTool, verilen aracı çağırır.
	CallTool(ctx context.Context, name string, args map[string]any) (Result, error)
}
