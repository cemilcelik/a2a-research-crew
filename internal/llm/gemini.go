package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const defaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// gemini, Google Gemini generateContent API'sini kullanan bir Provider
// uygulamasıdır.
type gemini struct {
	opts   Options
	client *http.Client
}

func newGemini(opts Options) *gemini {
	return &gemini{opts: opts, client: httpClient(opts)}
}

// Name implements Provider.
func (g *gemini) Name() string { return ProviderGemini }

// Complete implements Provider.
func (g *gemini) Complete(ctx context.Context, req Request) (Response, error) {
	model := g.opts.model(req.Model)
	if model == "" {
		return Response{}, fmt.Errorf("llm: gemini model must not be empty")
	}

	payload := geminiRequest{
		Contents: toGeminiContents(req.Messages),
		Tools:    toGeminiTools(req.Tools),
	}
	if req.System != "" {
		payload.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: req.System}},
		}
	}
	if req.Temperature != nil || req.MaxTokens > 0 {
		payload.GenerationConfig = &geminiGenerationConfig{
			Temperature:     req.Temperature,
			MaxOutputTokens: req.MaxTokens,
		}
	}

	baseURL := g.opts.BaseURL
	if baseURL == "" {
		baseURL = defaultGeminiBaseURL
	}
	url := fmt.Sprintf("%s/models/%s:generateContent", strings.TrimRight(baseURL, "/"), model)

	data, err := postJSON(ctx, g.client, url, map[string]string{
		"x-goog-api-key": g.opts.APIKey,
	}, payload)
	if err != nil {
		return Response{}, err
	}

	var parsed geminiResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return Response{}, fmt.Errorf("llm: decode gemini response: %w", err)
	}
	if len(parsed.Candidates) == 0 {
		return Response{}, fmt.Errorf("llm: gemini response has no candidates")
	}

	candidate := parsed.Candidates[0]
	resp := Response{
		FinishReason: candidate.FinishReason,
		Usage: Usage{
			InputTokens:  parsed.UsageMetadata.PromptTokenCount,
			OutputTokens: parsed.UsageMetadata.CandidatesTokenCount,
		},
	}
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			resp.Content += part.Text
		}
		if part.FunctionCall != nil {
			args, err := json.Marshal(part.FunctionCall.Args)
			if err != nil {
				return Response{}, fmt.Errorf("llm: encode gemini tool arguments: %w", err)
			}
			id := part.FunctionCall.Name
			if id == "" {
				id = fmt.Sprintf("call_%d", len(resp.ToolCalls))
			}
			resp.ToolCalls = append(resp.ToolCalls, ToolCall{
				ID:        id,
				Name:      part.FunctionCall.Name,
				Arguments: args,
			})
		}
	}
	return resp, nil
}

type geminiRequest struct {
	SystemInstruction *geminiContent          `json:"systemInstruction,omitempty"`
	Contents          []geminiContent         `json:"contents"`
	Tools             []geminiTool            `json:"tools,omitempty"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string              `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResp `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	Name string `json:"name"`
	Args any    `json:"args,omitempty"`
}

type geminiFunctionResp struct {
	Name     string `json:"name"`
	Response any    `json:"response"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDecl `json:"functionDeclarations"`
}

type geminiFunctionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type geminiGenerationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content      geminiContent `json:"content"`
		FinishReason string        `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
}

func toGeminiContents(messages []Message) []geminiContent {
	out := make([]geminiContent, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case RoleSystem:
			continue
		case RoleTool:
			name := m.Name
			if name == "" {
				name = m.ToolCallID
			}
			out = append(out, geminiContent{
				Role: string(RoleUser),
				Parts: []geminiPart{{
					FunctionResponse: &geminiFunctionResp{
						Name:     name,
						Response: map[string]any{"result": m.Content},
					},
				}},
			})
		case RoleAssistant:
			parts := make([]geminiPart, 0, len(m.ToolCalls)+1)
			if m.Content != "" {
				parts = append(parts, geminiPart{Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				parts = append(parts, geminiPart{FunctionCall: &geminiFunctionCall{
					Name: tc.Name,
					Args: rawJSONToAny(tc.Arguments),
				}})
			}
			out = append(out, geminiContent{Role: "model", Parts: parts})
		default:
			out = append(out, geminiContent{
				Role:  string(RoleUser),
				Parts: []geminiPart{{Text: m.Content}},
			})
		}
	}
	return out
}

func toGeminiTools(defs []ToolDef) []geminiTool {
	if len(defs) == 0 {
		return nil
	}
	decls := make([]geminiFunctionDecl, 0, len(defs))
	for _, d := range defs {
		decls = append(decls, geminiFunctionDecl{
			Name:        d.Name,
			Description: d.Description,
			Parameters:  d.Parameters,
		})
	}
	return []geminiTool{{FunctionDeclarations: decls}}
}

// rawJSONToAny, ham JSON'u genel bir değere çevirir; geçersizse boş harita
// döner.
func rawJSONToAny(raw []byte) any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return map[string]any{}
	}
	return v
}
