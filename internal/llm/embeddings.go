package llm

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
)

// Embedder, metinleri anlamsal vektörlere dönüştürür.
type Embedder interface {
	// Dimensions, üretilen vektörlerin boyutudur.
	Dimensions() int
	// Embed, verilen metinleri vektörlere dönüştürür.
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// MockEmbedder, ağ erişimi gerektirmeyen deterministik bir Embedder'dır.
type MockEmbedder struct {
	dims int
}

// NewMockEmbedder, verilen boyutta bir MockEmbedder oluşturur.
func NewMockEmbedder(dims int) *MockEmbedder {
	if dims <= 0 {
		dims = 64
	}
	return &MockEmbedder{dims: dims}
}

// Dimensions implements Embedder.
func (m *MockEmbedder) Dimensions() int { return m.dims }

// Embed implements Embedder.
func (m *MockEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for _, text := range texts {
		out = append(out, hashToVector(text, m.dims))
	}
	return out, nil
}

// hashToVector, bir metni deterministik olarak normalize edilmiş bir vektöre
// çevirir.
func hashToVector(text string, dims int) []float32 {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(text))))
	vec := make([]float32, dims)
	for i := range vec {
		b := sum[i%len(sum)]
		vec[i] = float32(b)/127.5 - 1.0
	}
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return vec
	}
	for i := range vec {
		vec[i] = float32(float64(vec[i]) / norm)
	}
	return vec
}

// OpenAIEmbedder, OpenAI Embeddings API'sini (ve uyumlu ağ geçitlerini)
// kullanan bir Embedder'dır.
type OpenAIEmbedder struct {
	opts    Options
	model   string
	dims    int
	baseURL string
	client  *http.Client
}

// NewOpenAIEmbedder, OpenAI uyumlu bir embeddings istemcisi oluşturur.
func NewOpenAIEmbedder(opts Options, model string, dims int) *OpenAIEmbedder {
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	return &OpenAIEmbedder{
		opts:    opts,
		model:   model,
		dims:    dims,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  httpClient(opts),
	}
}

// Dimensions implements Embedder.
func (e *OpenAIEmbedder) Dimensions() int { return e.dims }

// Embed implements Embedder.
func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	payload := map[string]any{"model": e.model, "input": texts}
	data, err := postJSON(ctx, e.client, e.baseURL+"/embeddings", map[string]string{
		"Authorization": "Bearer " + e.opts.APIKey,
	}, payload)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("llm: decode embeddings response: %w", err)
	}
	out := make([][]float32, 0, len(parsed.Data))
	for _, item := range parsed.Data {
		out = append(out, item.Embedding)
	}
	return out, nil
}
