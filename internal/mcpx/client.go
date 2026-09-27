// Package mcpx, resmi MCP Go SDK'sı üzerine ince bir istemci sarmalayıcısı
// sunar. Ajanlar MCP sunucularındaki araçları bu sarmalayıcı üzerinden keşfeder
// ve çağırır; araç tanımları LLM araç çağrısı formatına dönüştürülür.
package mcpx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"a2a-research-crew/internal/buildinfo"
	"a2a-research-crew/internal/llm"
)

// defaultInputSchema, şeması olmayan araçlar için kullanılan boş JSON
// Schema'dır.
var defaultInputSchema = []byte(`{"type":"object","properties":{}}`)

// ServerConfig, bir MCP sunucusuna bağlanmak için gereken yapılandırmadır.
// Command (stdio) veya URL (streamable HTTP) alanlarından biri doldurulmalıdır.
type ServerConfig struct {
	Name    string
	Command string
	Args    []string
	Env     []string
	URL     string
	Headers map[string]string
	Client  *http.Client
}

// Tool, MCP sunucusundan alınan bir araç tanımıdır.
type Tool struct {
	Name        string
	Description string
	InputSchema []byte
}

// Result, bir araç çağrısının sonucudur.
type Result struct {
	Text       string
	Structured any
	IsError    bool
}

// Client, tek bir MCP oturumunu temsil eder.
type Client struct {
	name    string
	session *sdk.ClientSession
}

// Connect, verilen yapılandırmaya göre gerçek bir MCP sunucusuna bağlanır.
func Connect(ctx context.Context, cfg ServerConfig) (*Client, error) {
	var transport sdk.Transport

	switch {
	case cfg.URL != "":
		client := cfg.Client
		if client == nil {
			client = http.DefaultClient
		}
		if len(cfg.Headers) > 0 {
			client = &http.Client{Transport: &headerRoundTripper{
				base:    client.Transport,
				headers: cfg.Headers,
			}}
		}
		transport = &sdk.StreamableClientTransport{Endpoint: cfg.URL, HTTPClient: client}
	case cfg.Command != "":
		cmd := exec.Command(cfg.Command, cfg.Args...)
		cmd.Env = append(os.Environ(), cfg.Env...)
		transport = &sdk.CommandTransport{Command: cmd}
	default:
		return nil, fmt.Errorf("mcpx: server %q requires either Command or URL", cfg.Name)
	}

	return ConnectTransport(ctx, cfg.Name, transport)
}

// ConnectTransport, verilen transport üzerinden bir istemci oturumu açar.
// Testlerde ve in-process sunucularda kullanılır.
func ConnectTransport(ctx context.Context, name string, transport sdk.Transport) (*Client, error) {
	client := sdk.NewClient(&sdk.Implementation{
		Name:    "a2a-research-crew",
		Version: buildinfo.Version,
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcpx: connect to %q: %w", name, err)
	}
	return &Client{name: name, session: session}, nil
}

// Name, MCP sunucusunun adını döner.
func (c *Client) Name() string { return c.name }

// Session, altta yatan SDK oturumunu döner (ileri düzey kullanım için).
func (c *Client) Session() *sdk.ClientSession { return c.session }

// ListTools, sunucudaki tüm araçları sayfalama ile birlikte listeler.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	var tools []Tool
	var cursor string
	for {
		res, err := c.session.ListTools(ctx, &sdk.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, fmt.Errorf("mcpx: list tools from %q: %w", c.name, err)
		}
		for _, t := range res.Tools {
			schema, err := json.Marshal(t.InputSchema)
			if err != nil {
				return nil, fmt.Errorf("mcpx: encode tool schema for %q: %w", t.Name, err)
			}
			if len(schema) == 0 || string(schema) == "null" {
				schema = defaultInputSchema
			}
			tools = append(tools, Tool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: schema,
			})
		}
		if res.NextCursor == "" {
			break
		}
		cursor = res.NextCursor
	}
	return tools, nil
}

// ToolDefs, sunucudaki araçları LLM araç tanımlarına dönüştürür.
func (c *Client) ToolDefs(ctx context.Context) ([]llm.ToolDef, error) {
	tools, err := c.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	defs := make([]llm.ToolDef, 0, len(tools))
	for _, t := range tools {
		defs = append(defs, llm.ToolDef{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.InputSchema,
		})
	}
	return defs, nil
}

// CallTool, sunucudaki bir aracı çağırır ve metinsel sonucu döner.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (Result, error) {
	res, err := c.session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return Result{}, fmt.Errorf("mcpx: call tool %q on %q: %w", name, c.name, err)
	}

	result := Result{Structured: res.StructuredContent, IsError: res.IsError}
	for _, content := range res.Content {
		if text, ok := content.(*sdk.TextContent); ok {
			result.Text += text.Text
		}
	}
	return result, nil
}

// Close, MCP oturumunu kapatır.
func (c *Client) Close() error {
	if c.session == nil {
		return nil
	}
	return c.session.Close()
}

type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	base := h.base
	if base == nil {
		base = http.DefaultTransport
	}
	clone := req.Clone(req.Context())
	for k, v := range h.headers {
		clone.Header.Set(k, v)
	}
	return base.RoundTrip(clone)
}
