// Package memory, ajanların kısa vadeli (bağlam içi konuşma) ve uzun vadeli
// (anlamsal, pgvector tabanlı) hafızasını yönetir.
package memory

import (
	"context"
	"time"

	"a2a-research-crew/internal/llm"
)

// MessageStore, bağlam (context) bazlı konuşma geçmişini saklar.
type MessageStore interface {
	// Append, verilen bağlama mesajları ekler.
	Append(ctx context.Context, contextID string, msgs ...llm.Message) error
	// Recent, bağlamın en son mesajlarını eskiden yeniye döner.
	Recent(ctx context.Context, contextID string, limit int) ([]llm.Message, error)
}

// Memory, uzun vadeli anlamsal bir hafıza kaydıdır.
type Memory struct {
	ID        string
	Content   string
	Metadata  map[string]any
	Score     float64
	CreatedAt time.Time
}

// SemanticStore, anlamsal (vektör tabanlı) hafızayı yönetir.
type SemanticStore interface {
	// Remember, bir metni gömülerek (embed) saklar ve kayıt kimliğini döner.
	Remember(ctx context.Context, namespace, content string, metadata map[string]any) (string, error)
	// Recall, sorguya en çok benzeyen kayıtları döner.
	Recall(ctx context.Context, namespace, query string, limit int) ([]Memory, error)
}

// Store, kısa ve uzun vadeli hafızayı birlikte sunar.
type Store interface {
	MessageStore
	SemanticStore
}
