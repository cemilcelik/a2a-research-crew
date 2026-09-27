package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// defaultOpenAIBaseURL, OpenAI'nin resmi API köküdür.
const defaultOpenAIBaseURL = "https://api.openai.com/v1"

// openAI, OpenAI Chat Completions API'sini (ve uyumlu ağ geçitlerini) kullanan
// bir Provider uygulamasıdır.
type openAI struct {
	opts    Options
	baseURL string
	client  *http.Client
}

func newOpenAI(opts Options, baseURL string) *openAI {
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	return &openAI{
		opts:    opts,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  httpClient(opts),
	}
}

// Name implements Provider.
func (o *openAI) Name() string {
	if o.opts.Provider != "" {
		return o.opts.Provider
	}
	return ProviderOpenAI
}

// Complete implements Provider.
func (o *openAI) Complete(ctx context.Context, req Request) (Response, error) {
	payload := openAIChatRequest{
		Model:       o.opts.model(req.Model),
		Messages:    toOpenAIMessages(req),
		Tools:       toOpenAITools(req.Tools),
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}

	data, err := postJSON(ctx, o.client, o.baseURL+"/chat/completions", map[string]string{
		"Authorization": "Bearer " + o.opts.APIKey,
	}, payload)
	if err != nil {
		return Response{}, err
	}

	var parsed openAIChatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return Response{}, fmt.Errorf("llm: decode openai response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return Response{}, fmt.Errorf("llm: openai response has no choices")
	}

	choice := parsed.Choices[0]
	resp := Response{
		Content:      choice.Message.Content,
		FinishReason: choice.FinishReason,
		Usage: Usage{
			InputTokens:  parsed.Usage.PromptTokens,
			OutputTokens: parsed.Usage.CompletionTokens,
		},
	}
	for _, tc := range choice.Message.ToolCalls {
		resp.ToolCalls = append(resp.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: []byte(tc.Function.Arguments),
		})
	}
	return resp, nil
}

type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Tools       []openAITool    `json:"tools,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	Name       string           `json:"name,omitempty"`
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIFunctionCall `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAITool struct {
	Type     string            `json:"type"`
	Function openAIFunctionDef `json:"function"`
}

type openAIFunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func toOpenAIMessages(req Request) []openAIMessage {
	var out []openAIMessage
	if req.System != "" {
		out = append(out, openAIMessage{Role: string(RoleSystem), Content: req.System})
	}
	for _, m := range req.Messages {
		msg := openAIMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		for _, tc := range m.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, openAIToolCall{
				ID:       tc.ID,
				Type:     "function",
				Function: openAIFunctionCall{Name: tc.Name, Arguments: string(tc.Arguments)},
			})
		}
		out = append(out, msg)
	}
	return out
}

func toOpenAITools(defs []ToolDef) []openAITool {
	if len(defs) == 0 {
		return nil
	}
	out := make([]openAITool, 0, len(defs))
	for _, d := range defs {
		out = append(out, openAITool{
			Type: "function",
			Function: openAIFunctionDef{
				Name:        d.Name,
				Description: d.Description,
				Parameters:  d.Parameters,
			},
		})
	}
	return out
}
