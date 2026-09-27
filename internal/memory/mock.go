package memory

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"a2a-research-crew/internal/llm"
)

// MockStore, ağ veya veritabanı gerektirmeyen in-memory bir Store
// uygulamasıdır. Testlerde ve dry-run çalışmalarında kullanılır.
type MockStore struct {
	embedder llm.Embedder

	mu       sync.Mutex
	messages map[string][]llm.Message
	entries  []mockEntry
}

type mockEntry struct {
	memory    Memory
	namespace string
	vector    []float32
}

// NewMockStore, verilen embedder ile bir MockStore oluşturur. Embedder nil ise
// 64 boyutlu MockEmbedder kullanılır.
func NewMockStore(embedder llm.Embedder) *MockStore {
	if embedder == nil {
		embedder = llm.NewMockEmbedder(64)
	}
	return &MockStore{
		embedder: embedder,
		messages: make(map[string][]llm.Message),
	}
}

// Append implements MessageStore.
func (m *MockStore) Append(_ context.Context, contextID string, msgs ...llm.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages[contextID] = append(m.messages[contextID], msgs...)
	return nil
}

// Recent implements MessageStore.
func (m *MockStore) Recent(_ context.Context, contextID string, limit int) ([]llm.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	all := m.messages[contextID]
	if limit <= 0 || limit > len(all) {
		limit = len(all)
	}
	out := make([]llm.Message, limit)
	copy(out, all[len(all)-limit:])
	return out, nil
}

// Remember implements SemanticStore.
func (m *MockStore) Remember(ctx context.Context, namespace, content string, metadata map[string]any) (string, error) {
	vecs, err := m.embedder.Embed(ctx, []string{content})
	if err != nil {
		return "", err
	}
	entry := mockEntry{memory: Memory{Content: content, Metadata: metadata, CreatedAt: time.Now()}, namespace: namespace}
	if len(vecs) > 0 {
		entry.vector = vecs[0]
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	entry.memory.ID = fmt.Sprintf("mem-%d", len(m.entries))
	m.entries = append(m.entries, entry)
	return entry.memory.ID, nil
}

// Recall implements SemanticStore.
func (m *MockStore) Recall(ctx context.Context, namespace, query string, limit int) ([]Memory, error) {
	vecs, err := m.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	var queryVec []float32
	if len(vecs) > 0 {
		queryVec = vecs[0]
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var scored []Memory
	for _, entry := range m.entries {
		if entry.namespace != namespace {
			continue
		}
		mem := entry.memory
		mem.Score = cosineSimilarity(queryVec, entry.vector)
		scored = append(scored, mem)
	}
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })
	if limit > 0 && len(scored) > limit {
		scored = scored[:limit]
	}
	return scored, nil
}

func makeID(seq int) string {
	return "mem-" + itoa(seq)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
