package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"a2a-research-crew/internal/llm"
)

// schemaTemplate, vektör boyutuna göre biçimlendirilen şema şablonudur.
// %d yerine embedder boyutu yazılır.
const schemaTemplate = `
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS memory_messages (
    context_id   text NOT NULL,
    seq          bigserial PRIMARY KEY,
    role         text NOT NULL,
    content      text NOT NULL,
    name         text,
    tool_call_id text,
    tool_calls   jsonb,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS memory_messages_context_idx ON memory_messages (context_id, seq);

CREATE TABLE IF NOT EXISTS memory_semantic (
    id         uuid PRIMARY KEY,
    namespace  text NOT NULL,
    content    text NOT NULL,
    embedding  vector(%d) NOT NULL,
    metadata   jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS memory_semantic_namespace_idx ON memory_semantic (namespace);
`

// PostgresStore, Store arayüzünün PostgreSQL + pgvector uygulamasıdır.
type PostgresStore struct {
	pool     *pgxpool.Pool
	embedder llm.Embedder
}

var _ Store = (*PostgresStore)(nil)

// NewPostgresStore, verilen DSN ile bağlanır. Embedder, uzun vadeli hafızanın
// vektör boyutunu belirler.
func NewPostgresStore(ctx context.Context, dsn string, embedder llm.Embedder) (*PostgresStore, error) {
	if embedder == nil {
		return nil, fmt.Errorf("memory: embedder is required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("memory: create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("memory: ping database: %w", err)
	}
	return &PostgresStore{pool: pool, embedder: embedder}, nil
}

// Migrate, vektör boyutuna uygun şemayı (idempotent olarak) oluşturur.
func (s *PostgresStore) Migrate(ctx context.Context) error {
	schema := fmt.Sprintf(schemaTemplate, s.embedder.Dimensions())
	if _, err := s.pool.Exec(ctx, schema); err != nil {
		return fmt.Errorf("memory: apply schema: %w", err)
	}
	return nil
}

// Close, bağlantı havuzunu kapatır.
func (s *PostgresStore) Close() { s.pool.Close() }

// Append implements MessageStore.
func (s *PostgresStore) Append(ctx context.Context, contextID string, msgs ...llm.Message) error {
	if len(msgs) == 0 {
		return nil
	}
	const query = `
INSERT INTO memory_messages (context_id, role, content, name, tool_call_id, tool_calls)
VALUES ($1, $2, $3, $4, $5, $6)`

	batch := &pgx.Batch{}
	for _, m := range msgs {
		toolCalls, err := marshalToolCalls(m.ToolCalls)
		if err != nil {
			return err
		}
		batch.Queue(query, contextID, string(m.Role), m.Content, m.Name, m.ToolCallID, toolCalls)
	}

	results := s.pool.SendBatch(ctx, batch)
	defer func() { _ = results.Close() }()
	for range msgs {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("memory: append message: %w", err)
		}
	}
	return nil
}

// Recent implements MessageStore.
func (s *PostgresStore) Recent(ctx context.Context, contextID string, limit int) ([]llm.Message, error) {
	if limit <= 0 {
		limit = 20
	}
	const query = `
SELECT role, content, name, tool_call_id, tool_calls
FROM memory_messages
WHERE context_id = $1
ORDER BY seq DESC
LIMIT $2`

	rows, err := s.pool.Query(ctx, query, contextID, limit)
	if err != nil {
		return nil, fmt.Errorf("memory: query messages: %w", err)
	}
	defer rows.Close()

	var reversed []llm.Message
	for rows.Next() {
		var (
			role, content string
			name, callID  *string
			toolCallsRaw  []byte
		)
		if err := rows.Scan(&role, &content, &name, &callID, &toolCallsRaw); err != nil {
			return nil, fmt.Errorf("memory: scan message: %w", err)
		}
		msg := llm.Message{Role: llm.Role(role), Content: content}
		if name != nil {
			msg.Name = *name
		}
		if callID != nil {
			msg.ToolCallID = *callID
		}
		calls, err := unmarshalToolCalls(toolCallsRaw)
		if err != nil {
			return nil, err
		}
		msg.ToolCalls = calls
		reversed = append(reversed, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: iterate messages: %w", err)
	}

	// Sorgu yeniden eskiye döndüğü için ters çeviririz.
	out := make([]llm.Message, len(reversed))
	for i, msg := range reversed {
		out[len(reversed)-1-i] = msg
	}
	return out, nil
}

// Remember implements SemanticStore.
func (s *PostgresStore) Remember(ctx context.Context, namespace, content string, metadata map[string]any) (string, error) {
	vectors, err := s.embedder.Embed(ctx, []string{content})
	if err != nil {
		return "", fmt.Errorf("memory: embed content: %w", err)
	}
	if len(vectors) == 0 {
		return "", fmt.Errorf("memory: embedder returned no vector")
	}

	meta, err := marshalMetadata(metadata)
	if err != nil {
		return "", err
	}

	id := uuid.NewString()
	const query = `
INSERT INTO memory_semantic (id, namespace, content, embedding, metadata)
VALUES ($1, $2, $3, ($4::text)::vector, $5)`
	if _, err := s.pool.Exec(ctx, query, id, namespace, content, encodeVector(vectors[0]), meta); err != nil {
		return "", fmt.Errorf("memory: insert memory: %w", err)
	}
	return id, nil
}

// Recall implements SemanticStore.
func (s *PostgresStore) Recall(ctx context.Context, namespace, query string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 5
	}
	vectors, err := s.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("memory: embed query: %w", err)
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("memory: embedder returned no vector")
	}

	const stmt = `
SELECT id, content, metadata, 1 - (embedding <=> ($1::text)::vector) AS score, created_at
FROM memory_semantic
WHERE namespace = $2
ORDER BY embedding <=> ($1::text)::vector
LIMIT $3`

	rows, err := s.pool.Query(ctx, stmt, encodeVector(vectors[0]), namespace, limit)
	if err != nil {
		return nil, fmt.Errorf("memory: recall memories: %w", err)
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		var (
			id        string
			content   string
			metaRaw   []byte
			score     float64
			createdAt time.Time
		)
		if err := rows.Scan(&id, &content, &metaRaw, &score, &createdAt); err != nil {
			return nil, fmt.Errorf("memory: scan memory: %w", err)
		}
		meta, err := unmarshalMetadata(metaRaw)
		if err != nil {
			return nil, err
		}
		memories = append(memories, Memory{
			ID:        id,
			Content:   content,
			Metadata:  meta,
			Score:     score,
			CreatedAt: createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: iterate memories: %w", err)
	}
	return memories, nil
}

func encodeVector(vec []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, v := range vec {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(v), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

func marshalToolCalls(calls []llm.ToolCall) ([]byte, error) {
	if len(calls) == 0 {
		return nil, nil
	}
	raw, err := json.Marshal(calls)
	if err != nil {
		return nil, fmt.Errorf("memory: encode tool calls: %w", err)
	}
	return raw, nil
}

func unmarshalToolCalls(raw []byte) ([]llm.ToolCall, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var calls []llm.ToolCall
	if err := json.Unmarshal(raw, &calls); err != nil {
		return nil, fmt.Errorf("memory: decode tool calls: %w", err)
	}
	return calls, nil
}

func marshalMetadata(metadata map[string]any) ([]byte, error) {
	if metadata == nil {
		return nil, nil
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("memory: encode metadata: %w", err)
	}
	return raw, nil
}

func unmarshalMetadata(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("memory: decode metadata: %w", err)
	}
	return meta, nil
}
