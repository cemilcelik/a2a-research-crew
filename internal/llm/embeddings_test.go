package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMockEmbedderDeterministic(t *testing.T) {
	e := NewMockEmbedder(8)
	ctx := context.Background()

	first, err := e.Embed(ctx, []string{"hello world"})
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}
	second, _ := e.Embed(ctx, []string{"hello world"})
	if len(first) != 1 || len(first[0]) != 8 {
		t.Fatalf("embedding shape = %dx?, want 1x8", len(first))
	}
	for i := range first[0] {
		if first[0][i] != second[0][i] {
			t.Fatalf("embedding not deterministic at %d: %v vs %v", i, first[0][i], second[0][i])
		}
	}

	other, _ := e.Embed(ctx, []string{"different text"})
	if sameVector(first[0], other[0]) {
		t.Error("different texts produced identical embeddings")
	}
}

func TestOpenAIEmbedder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("path = %q, want /embeddings", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("auth = %q, want Bearer secret", r.Header.Get("Authorization"))
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "text-embedding-test" {
			t.Errorf("model = %v, want text-embedding-test", body["model"])
		}
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	}))
	defer srv.Close()

	e := NewOpenAIEmbedder(Options{APIKey: "secret", BaseURL: srv.URL}, "text-embedding-test", 3)
	if e.Dimensions() != 3 {
		t.Errorf("Dimensions() = %d, want 3", e.Dimensions())
	}
	got, err := e.Embed(context.Background(), []string{"hi"})
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}
	if len(got) != 1 || len(got[0]) != 3 || got[0][2] != 0.3 {
		t.Fatalf("Embed() = %+v, want [[0.1 0.2 0.3]]", got)
	}
}

func sameVector(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
