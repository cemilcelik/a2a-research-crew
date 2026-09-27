// Package llm, sağlayıcıdan bağımsız bir LLM soyutlaması sunar. Ajanlar bu
// arayüz üzerinden konuşur; gerçek sağlayıcılar (OpenAI, Anthropic, Gemini,
// OpenAI-uyumlu ağ geçitleri) ve testler için deterministik mock adaptörleri
// aynı sözleşmeyi uygular.
package llm

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"time"
)

// Sağlayıcı adları.
const (
	ProviderMock             = "mock"
	ProviderOpenAI           = "openai"
	ProviderAnthropic        = "anthropic"
	ProviderGemini           = "gemini"
	ProviderOpenAICompatible = "openai-compatible"
)

// ErrMissingAPIKey, gerçek bir sağlayıcı için anahtar verilmediğinde döner.
var ErrMissingAPIKey = errors.New("llm: missing api key")

// Role, bir mesajın konuşmadaki rolüdür.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message, sağlayıcıdan bağımsız tek bir konuşma mesajıdır.
type Message struct {
	// Role, mesajın sahibidir.
	Role Role
	// Content, metin içeriğidir.
	Content string
	// ToolCalls, assistant mesajının talep ettiği araç çağrılarıdır.
	ToolCalls []ToolCall
	// ToolCallID, RoleTool mesajının yanıtladığı çağrının kimliğidir.
	ToolCallID string
	// Name, RoleTool için araç adıdır (opsiyonel).
	Name string
}

// ToolCall, modelin talep ettiği bir araç çağrısıdır.
type ToolCall struct {
	ID        string
	Name      string
	Arguments []byte // ham JSON
}

// ToolDef, modele sunulan bir aracın tanımıdır.
type ToolDef struct {
	Name        string
	Description string
	// Parameters, aracın argümanları için JSON Schema'dır.
	Parameters []byte
}

// Request, bir tamamlama isteğidir.
type Request struct {
	// Model, kullanılacak model kimliğidir. Boşsa sağlayıcının varsayılanı
	// kullanılır.
	Model string
	// System, sistem talimatıdır.
	System string
	// Messages, konuşma geçmişidir.
	Messages []Message
	// Tools, modele sunulacak araçlardır.
	Tools []ToolDef
	// Temperature, örnekleme sıcaklığıdır (nil ise sağlayıcı varsayılanı).
	Temperature *float64
	// MaxTokens, üretilecek en fazla token sayısıdır (0 ise sağlayıcı
	// varsayılanı).
	MaxTokens int
}

// Usage, token tüketimini bildirir.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Response, bir tamamlama sonucudur.
type Response struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
	Usage        Usage
}

// Provider, tamamlama yeteneği olan bir LLM sağlayıcısıdır.
type Provider interface {
	// Name, sağlayıcının adını döner.
	Name() string
	// Complete, verilen istek için tek bir tamamlama üretir.
	Complete(ctx context.Context, req Request) (Response, error)
}

// Chunk, akış (streaming) sırasında üretilen kısmi çıktıdır.
type Chunk struct {
	// Delta, metin parçasıdır.
	Delta string
	// ToolCallDelta, akış içinde gelen araç çağrısı parçasıdır.
	ToolCallDelta *ToolCall
	// Usage, akış sonunda doldurulur.
	Usage *Usage
	// Done, akışın bittiğini bildirir.
	Done bool
}

// StreamingProvider, tamamlamayı parça parça üretebilen sağlayıcıdır.
type StreamingProvider interface {
	Provider
	// Stream, verilen istek için kısmi çıktılar üretir.
	Stream(ctx context.Context, req Request) iter.Seq2[Chunk, error]
}

// Options, bir sağlayıcı örneği oluşturmak için gereken yapılandırmadır.
type Options struct {
	Provider     string
	APIKey       string
	BaseURL      string
	DefaultModel string
	Timeout      time.Duration
	HTTPClient   *http.Client
}

// New, verilen seçeneklere göre bir Provider oluşturur.
func New(opts Options) (Provider, error) {
	provider := opts.Provider
	if provider == "" {
		provider = ProviderMock
	}

	switch provider {
	case ProviderMock:
		return NewMock(opts.DefaultModel), nil
	case ProviderOpenAI:
		if opts.APIKey == "" {
			return nil, fmt.Errorf("%w for provider %q", ErrMissingAPIKey, provider)
		}
		return newOpenAI(opts, opts.BaseURL), nil
	case ProviderOpenAICompatible:
		if opts.APIKey == "" {
			return nil, fmt.Errorf("%w for provider %q", ErrMissingAPIKey, provider)
		}
		if opts.BaseURL == "" {
			return nil, fmt.Errorf("llm: base URL is required for provider %q", provider)
		}
		return newOpenAI(opts, opts.BaseURL), nil
	case ProviderAnthropic:
		if opts.APIKey == "" {
			return nil, fmt.Errorf("%w for provider %q", ErrMissingAPIKey, provider)
		}
		return newAnthropic(opts), nil
	case ProviderGemini:
		if opts.APIKey == "" {
			return nil, fmt.Errorf("%w for provider %q", ErrMissingAPIKey, provider)
		}
		return newGemini(opts), nil
	default:
		return nil, fmt.Errorf("llm: unknown provider %q", provider)
	}
}

func (o Options) model(reqModel string) string {
	if reqModel != "" {
		return reqModel
	}
	return o.DefaultModel
}
