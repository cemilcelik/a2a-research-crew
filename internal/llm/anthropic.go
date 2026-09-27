package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	defaultAnthropicBaseURL   = "https://api.anthropic.com/v1"
	anthropicVersionHeader    = "2023-06-01"
	defaultAnthropicMaxTokens = 1024
)

// anthropic, Anthropic Messages API'sini kullanan bir Provider uygulamasıdır.
type anthropic struct {
	opts   Options
	client *http.Client
}

func newAnthropic(opts Options) *anthropic {
	return &anthropic{opts: opts, client: httpClient(opts)}
}

// Name implements Provider.
func (a *anthropic) Name() string { return ProviderAnthropic }

// Complete implements Provider.
func (a *anthropic) Complete(ctx context.Context, req Request) (Response, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultAnthropicMaxTokens
	}

	payload := anthropicRequest{
		Model:       a.opts.model(req.Model),
		System:      req.System,
		MaxTokens:   maxTokens,
		Temperature: req.Temperature,
		Messages:    toAnthropicMessages(req.Messages),
		Tools:       toAnthropicTools(req.Tools),
	}
	if payload.Model == "" {
		return Response{}, fmt.Errorf("llm: anthropic model must not be empty")
	}

	baseURL := a.opts.BaseURL
	if baseURL == "" {
		baseURL = defaultAnthropicBaseURL
	}
	data, err := postJSON(ctx, a.client, strings.TrimRight(baseURL, "/")+"/messages", map[string]string{
		"x-api-key":         a.opts.APIKey,
		"anthropic-version": anthropicVersionHeader,
	}, payload)
	if err != nil {
		return Response{}, err
	}

	var parsed anthropicResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return Response{}, fmt.Errorf("llm: decode anthropic response: %w", err)
	}

	resp := Response{
		FinishReason: parsed.StopReason,
		Usage: Usage{
			InputTokens:  parsed.Usage.InputTokens,
			OutputTokens: parsed.Usage.OutputTokens,
		},
	}
	for _, block := range parsed.Content {
		switch block.Type {
		case "text":
			resp.Content += block.Text
		case "tool_use":
			resp.ToolCalls = append(resp.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: block.Input,
			})
		}
	}
	return resp, nil
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Tools       []anthropicTool    `json:"tools,omitempty"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicContent struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

type anthropicResponse struct {
	Content    []anthropicContent `json:"content"`
	StopReason string             `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func toAnthropicMessages(messages []Message) []anthropicMessage {
	out := make([]anthropicMessage, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case RoleSystem:
			// Sistem talimatı istek düzeyinde taşınır; mesaj dizisinde atlanır.
			continue
		case RoleTool:
			out = append(out, anthropicMessage{
				Role: string(RoleUser),
				Content: []anthropicContent{{
					Type:      "tool_result",
					ToolUseID: m.ToolCallID,
					Content:   m.Content,
				}},
			})
		case RoleAssistant:
			content := make([]anthropicContent, 0, len(m.ToolCalls)+1)
			if m.Content != "" {
				content = append(content, anthropicContent{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				content = append(content, anthropicContent{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: tc.Arguments,
				})
			}
			out = append(out, anthropicMessage{Role: string(RoleAssistant), Content: content})
		default:
			out = append(out, anthropicMessage{
				Role:    string(RoleUser),
				Content: []anthropicContent{{Type: "text", Text: m.Content}},
			})
		}
	}
	return out
}

func toAnthropicTools(defs []ToolDef) []anthropicTool {
	if len(defs) == 0 {
		return nil
	}
	out := make([]anthropicTool, 0, len(defs))
	for _, d := range defs {
		out = append(out, anthropicTool{
			Name:        d.Name,
			Description: d.Description,
			InputSchema: d.Parameters,
		})
	}
	return out
}
