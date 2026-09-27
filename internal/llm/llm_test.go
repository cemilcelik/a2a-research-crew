package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMockDefaultEcho(t *testing.T) {
	m := NewMock("mock-model")
	resp, err := m.Complete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if !strings.Contains(resp.Content, "hello") {
		t.Errorf("Content = %q, want it to contain the user message", resp.Content)
	}
}

func TestMockScripted(t *testing.T) {
	m := NewScripted(
		Response{Content: "first"},
		Response{Content: "second"},
	)
	ctx := context.Background()

	for i, want := range []string{"first", "second"} {
		resp, err := m.Complete(ctx, Request{})
		if err != nil {
			t.Fatalf("call %d: Complete() error: %v", i, err)
		}
		if resp.Content != want {
			t.Errorf("call %d: Content = %q, want %q", i, resp.Content, want)
		}
	}
}

func TestNewRequiresAPIKey(t *testing.T) {
	for _, provider := range []string{ProviderOpenAI, ProviderAnthropic, ProviderGemini, ProviderOpenAICompatible} {
		t.Run(provider, func(t *testing.T) {
			_, err := New(Options{Provider: provider})
			if err == nil {
				t.Fatalf("New() = nil error, want error for missing API key")
			}
		})
	}
}

func TestNewUnknownProvider(t *testing.T) {
	if _, err := New(Options{Provider: "nope"}); err == nil {
		t.Fatal("New() = nil error, want error for unknown provider")
	}
}

func TestOpenAICompatibleRequiresBaseURL(t *testing.T) {
	_, err := New(Options{Provider: ProviderOpenAICompatible, APIKey: "k"})
	if err == nil {
		t.Fatal("New() = nil error, want error for missing base URL")
	}
}

func TestOpenAIComplete(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["model"] != "gpt-test" {
			t.Errorf("model = %v, want gpt-test", body["model"])
		}
		_, _ = w.Write([]byte(`{
			"choices":[{"message":{"content":"hi there","tool_calls":[
				{"id":"call_1","type":"function","function":{"name":"search","arguments":"{\"q\":\"x\"}"}}
			]},"finish_reason":"tool_calls"}],
			"usage":{"prompt_tokens":3,"completion_tokens":5}
		}`))
	}))
	defer srv.Close()

	p, err := New(Options{Provider: ProviderOpenAI, APIKey: "secret", BaseURL: srv.URL, DefaultModel: "gpt-test"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	resp, err := p.Complete(context.Background(), Request{System: "be brief"})
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}

	if gotAuth != "Bearer secret" {
		t.Errorf("auth header = %q, want %q", gotAuth, "Bearer secret")
	}
	if resp.Content != "hi there" {
		t.Errorf("Content = %q, want %q", resp.Content, "hi there")
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "search" {
		t.Fatalf("ToolCalls = %+v, want one call to search", resp.ToolCalls)
	}
	if resp.Usage.OutputTokens != 5 {
		t.Errorf("OutputTokens = %d, want 5", resp.Usage.OutputTokens)
	}
}

func TestAnthropicComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "secret" {
			t.Errorf("x-api-key = %q, want secret", r.Header.Get("x-api-key"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["system"] != "sys" {
			t.Errorf("system = %v, want sys", body["system"])
		}
		_, _ = w.Write([]byte(`{
			"content":[{"type":"text","text":"analysis"},{"type":"tool_use","id":"tu_1","name":"calc","input":{"x":1}}],
			"stop_reason":"tool_use",
			"usage":{"input_tokens":4,"output_tokens":6}
		}`))
	}))
	defer srv.Close()

	p, err := New(Options{Provider: ProviderAnthropic, APIKey: "secret", BaseURL: srv.URL, DefaultModel: "claude-test"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	resp, err := p.Complete(context.Background(), Request{System: "sys"})
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if resp.Content != "analysis" {
		t.Errorf("Content = %q, want analysis", resp.Content)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "calc" {
		t.Fatalf("ToolCalls = %+v, want one call to calc", resp.ToolCalls)
	}
	if resp.Usage.InputTokens != 4 {
		t.Errorf("InputTokens = %d, want 4", resp.Usage.InputTokens)
	}
}

func TestGeminiComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "gemini-test:generateContent") {
			t.Errorf("path = %q, want it to contain model and generateContent", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "secret" {
			t.Errorf("x-goog-api-key = %q, want secret", r.Header.Get("x-goog-api-key"))
		}
		_, _ = w.Write([]byte(`{
			"candidates":[{"content":{"role":"model","parts":[
				{"text":"ok"},{"functionCall":{"name":"lookup","args":{"id":"1"}}}
			]},"finishReason":"STOP"}],
			"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3}
		}`))
	}))
	defer srv.Close()

	p, err := New(Options{Provider: ProviderGemini, APIKey: "secret", BaseURL: srv.URL, DefaultModel: "gemini-test"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	resp, err := p.Complete(context.Background(), Request{})
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("Content = %q, want ok", resp.Content)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "lookup" {
		t.Fatalf("ToolCalls = %+v, want one call to lookup", resp.ToolCalls)
	}
}

func TestOpenAIErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer srv.Close()

	p, _ := New(Options{Provider: ProviderOpenAI, APIKey: "secret", BaseURL: srv.URL, DefaultModel: "gpt-test"})
	if _, err := p.Complete(context.Background(), Request{}); err == nil {
		t.Fatal("Complete() = nil error, want provider error")
	}
}
