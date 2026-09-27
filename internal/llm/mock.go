package llm

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"sync"
)

// Mock, ağ erişimi gerektirmeyen, deterministik bir Provider uygulamasıdır.
// Testlerde ve gerçek API anahtarı olmadan yapılan dry-run çalışmalarında
// kullanılır.
type Mock struct {
	defaultModel string

	mu sync.Mutex
	// CompleteFunc, tanımlıysa tamamlama davranışını tamamen devralır.
	CompleteFunc func(ctx context.Context, req Request) (Response, error)
	// queue, sırayla döndürülecek hazır yanıtları tutar.
	queue []Response
}

// NewMock, varsayılan echo davranışına sahip bir Mock oluşturur.
func NewMock(defaultModel string) *Mock {
	if defaultModel == "" {
		defaultModel = "mock-model"
	}
	return &Mock{defaultModel: defaultModel}
}

// NewScripted, verilen yanıtları sırayla döndüren bir Mock oluşturur. Kuyruk
// tükendiğinde son yanıt tekrar eder. Araç çağrısı döngülerini test etmek için
// kullanışlıdır.
func NewScripted(responses ...Response) *Mock {
	return &Mock{defaultModel: "mock-model", queue: responses}
}

// Name implements Provider.
func (m *Mock) Name() string { return ProviderMock }

// Complete implements Provider.
func (m *Mock) Complete(ctx context.Context, req Request) (Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.CompleteFunc != nil {
		return m.CompleteFunc(ctx, req)
	}
	if len(m.queue) > 0 {
		resp := m.queue[0]
		m.queue = m.queue[1:]
		return resp, nil
	}

	// Varsayılan deterministik davranış: son kullanıcı mesajını yansıt.
	last := lastUserContent(req.Messages)
	if last == "" {
		last = "(empty)"
	}
	return Response{
		Content:      fmt.Sprintf("mock response for %q: %s", m.defaultModel, last),
		FinishReason: "stop",
		Usage:        Usage{InputTokens: len(req.Messages), OutputTokens: len(last)},
	}, nil
}

// Stream implements StreamingProvider.
func (m *Mock) Stream(ctx context.Context, req Request) iter.Seq2[Chunk, error] {
	return func(yield func(Chunk, error) bool) {
		resp, err := m.Complete(ctx, req)
		if err != nil {
			yield(Chunk{}, err)
			return
		}
		for _, word := range strings.Fields(resp.Content) {
			if !yield(Chunk{Delta: word + " "}, nil) {
				return
			}
		}
		if len(resp.ToolCalls) > 0 {
			yield(Chunk{ToolCallDelta: &resp.ToolCalls[0]}, nil)
		}
		usage := resp.Usage
		yield(Chunk{Usage: &usage, Done: true}, nil)
	}
}

func lastUserContent(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == RoleUser {
			return messages[i].Content
		}
	}
	return ""
}
