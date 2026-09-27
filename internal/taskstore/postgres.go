// Package taskstore, A2A görevlerinin PostgreSQL üzerinde kalıcı olarak
// saklanmasını sağlar. SDK'nın taskstore.Store sözleşmesini, iyimser
// eşzamanlılık kontrolü (OCC) ve kullanıcı izolasyonu ile uygular.
package taskstore

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	sdkstore "github.com/a2aproject/a2a-go/v2/a2asrv/taskstore"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// defaultPageSize, ListTasks için varsayılan sayfa boyutudur.
const defaultPageSize = 50

// maxPageSize, ListTasks için izin verilen en büyük sayfa boyutudur.
const maxPageSize = 100

// defaultMaxHistoryLength, ListTasks yanıtında dönen azami geçmiş uzunluğudur.
const defaultMaxHistoryLength = 100

// pgUniqueViolation, PostgreSQL benzersizlik ihlali hata kodudur.
const pgUniqueViolation = "23505"

const schemaSQL = `
CREATE TABLE IF NOT EXISTS a2a_tasks (
    id               text PRIMARY KEY,
    context_id       text NOT NULL,
    user_id          text NOT NULL DEFAULT '',
    state            text NOT NULL,
    status_timestamp timestamptz,
    version          bigint NOT NULL DEFAULT 1,
    task             jsonb NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS a2a_tasks_user_updated_idx ON a2a_tasks (user_id, updated_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS a2a_tasks_context_idx ON a2a_tasks (context_id);
`

// Postgres, sdkstore.Store arayüzünün PostgreSQL uygulamasıdır.
type Postgres struct {
	pool *pgxpool.Pool
	auth sdkstore.Authenticator
}

var _ sdkstore.Store = (*Postgres)(nil)

// NewPostgres, verilen DSN ile bir bağlantı havuzu oluşturur.
func NewPostgres(ctx context.Context, dsn string, auth sdkstore.Authenticator) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("taskstore: create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("taskstore: ping database: %w", err)
	}
	if auth == nil {
		auth = func(context.Context) (string, error) { return "", nil }
	}
	return &Postgres{pool: pool, auth: auth}, nil
}

// Migrate, gerekli şemayı (idempotent olarak) oluşturur.
func (s *Postgres) Migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, schemaSQL); err != nil {
		return fmt.Errorf("taskstore: apply schema: %w", err)
	}
	return nil
}

// Close, bağlantı havuzunu kapatır.
func (s *Postgres) Close() { s.pool.Close() }

// Create implements sdkstore.Store.
func (s *Postgres) Create(ctx context.Context, task *a2a.Task) (sdkstore.TaskVersion, error) {
	if err := validateTask(task); err != nil {
		return sdkstore.TaskVersionMissing, err
	}
	if task.ID == "" {
		return sdkstore.TaskVersionMissing, errors.New("taskstore: task id must not be empty")
	}

	user, err := s.authenticator(ctx)
	if err != nil {
		return sdkstore.TaskVersionMissing, err
	}
	payload, err := json.Marshal(task)
	if err != nil {
		return sdkstore.TaskVersionMissing, fmt.Errorf("taskstore: encode task: %w", err)
	}

	const query = `
INSERT INTO a2a_tasks (id, context_id, user_id, state, status_timestamp, version, task)
VALUES ($1, $2, $3, $4, $5, 1, $6)`
	_, err = s.pool.Exec(ctx, query,
		string(task.ID), task.ContextID, user, string(task.Status.State), statusTimestamp(task), payload,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return sdkstore.TaskVersionMissing, sdkstore.ErrTaskAlreadyExists
		}
		return sdkstore.TaskVersionMissing, fmt.Errorf("taskstore: create task: %w", err)
	}
	return sdkstore.TaskVersion(1), nil
}

// Update implements sdkstore.Store.
func (s *Postgres) Update(ctx context.Context, update *sdkstore.UpdateRequest) (sdkstore.TaskVersion, error) {
	if update == nil || update.Task == nil {
		return sdkstore.TaskVersionMissing, errors.New("taskstore: update requires a task")
	}
	if err := validateTask(update.Task); err != nil {
		return sdkstore.TaskVersionMissing, err
	}

	user, err := s.authenticator(ctx)
	if err != nil {
		return sdkstore.TaskVersionMissing, err
	}
	payload, err := json.Marshal(update.Task)
	if err != nil {
		return sdkstore.TaskVersionMissing, fmt.Errorf("taskstore: encode task: %w", err)
	}

	var (
		query string
		args  []any
	)
	const baseUpdate = `
UPDATE a2a_tasks
SET task = $1, state = $2, status_timestamp = $3, version = version + 1, updated_at = now()
WHERE id = $4 AND user_id = $5`
	args = []any{payload, string(update.Task.Status.State), statusTimestamp(update.Task), string(update.Task.ID), user}

	query = baseUpdate
	if update.PrevVersion != sdkstore.TaskVersionMissing {
		query += " AND version = $6"
		args = append(args, int64(update.PrevVersion))
	}
	query += " RETURNING version"

	var newVersion int64
	err = s.pool.QueryRow(ctx, query, args...).Scan(&newVersion)
	if err == nil {
		return sdkstore.TaskVersion(newVersion), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return sdkstore.TaskVersionMissing, fmt.Errorf("taskstore: update task: %w", err)
	}

	// Güncelleme uygulanamadı: sahiplik/yokluk veya sürüm çakışmasını ayırt et.
	exists, verr := s.existsForUser(ctx, update.Task.ID, user)
	if verr != nil {
		return sdkstore.TaskVersionMissing, verr
	}
	if !exists {
		return sdkstore.TaskVersionMissing, a2a.ErrTaskNotFound
	}
	return sdkstore.TaskVersionMissing, sdkstore.ErrConcurrentModification
}

// Get implements sdkstore.Store.
func (s *Postgres) Get(ctx context.Context, taskID a2a.TaskID) (*sdkstore.StoredTask, error) {
	user, err := s.authenticator(ctx)
	if err != nil {
		return nil, err
	}

	const query = `SELECT task, user_id, version FROM a2a_tasks WHERE id = $1 AND user_id = $2`
	var (
		payload []byte
		owner   string
		version int64
	)
	err = s.pool.QueryRow(ctx, query, string(taskID), user).Scan(&payload, &owner, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, a2a.ErrTaskNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("taskstore: get task: %w", err)
	}

	task, err := decodeTask(payload)
	if err != nil {
		return nil, err
	}
	return &sdkstore.StoredTask{Task: task, Version: sdkstore.TaskVersion(version), User: owner}, nil
}

// List implements sdkstore.Store.
func (s *Postgres) List(ctx context.Context, req *a2a.ListTasksRequest) (*a2a.ListTasksResponse, error) {
	user, err := s.authenticator(ctx)
	if err != nil {
		return nil, err
	}
	if user == "" {
		return nil, a2a.ErrUnauthenticated
	}

	pageSize := req.PageSize
	if pageSize == 0 {
		pageSize = defaultPageSize
	} else if pageSize < 1 || pageSize > maxPageSize {
		return nil, fmt.Errorf("page size must be between 1 and %d inclusive, got %d: %w", maxPageSize, pageSize, a2a.ErrInvalidRequest)
	}

	args := []any{user}
	conditions := []string{"user_id = $1"}
	addCondition := func(column string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	if req.ContextID != "" {
		addCondition("context_id", req.ContextID)
	}
	if req.Status != a2a.TaskStateUnspecified {
		addCondition("state", string(req.Status))
	}
	if req.StatusTimestampAfter != nil {
		args = append(args, *req.StatusTimestampAfter)
		conditions = append(conditions, fmt.Sprintf("status_timestamp IS NOT NULL AND status_timestamp >= $%d", len(args)))
	}
	where := strings.Join(conditions, " AND ")

	var totalSize int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM a2a_tasks WHERE "+where, args...).Scan(&totalSize); err != nil {
		return nil, fmt.Errorf("taskstore: count tasks: %w", err)
	}

	pageArgs := slices.Clone(args)
	pageWhere := where
	if req.PageToken != "" {
		cursorTime, cursorID, err := decodePageToken(req.PageToken)
		if err != nil {
			return nil, err
		}
		pageArgs = append(pageArgs, cursorTime, string(cursorID))
		pageWhere += fmt.Sprintf(
			" AND (updated_at < $%d OR (updated_at = $%d AND id < $%d))",
			len(pageArgs)-1, len(pageArgs)-1, len(pageArgs),
		)
	}
	pageArgs = append(pageArgs, pageSize+1)
	query := fmt.Sprintf(
		"SELECT task, updated_at, id FROM a2a_tasks WHERE %s ORDER BY updated_at DESC, id DESC LIMIT $%d",
		pageWhere, len(pageArgs),
	)

	rows, err := s.pool.Query(ctx, query, pageArgs...)
	if err != nil {
		return nil, fmt.Errorf("taskstore: list tasks: %w", err)
	}
	defer rows.Close()

	var (
		tasks         []*a2a.Task
		nextPageToken string
		lastTime      time.Time
		lastID        a2a.TaskID
	)
	for rows.Next() {
		var (
			payload   []byte
			updatedAt time.Time
			id        string
		)
		if err := rows.Scan(&payload, &updatedAt, &id); err != nil {
			return nil, fmt.Errorf("taskstore: scan task: %w", err)
		}
		// Sayfa dolduğunda okunan fazladan satır yalnızca "devamı var"
		// sinyalidir; cursor sayfanın son satırından alınır.
		if len(tasks) == pageSize {
			nextPageToken = encodePageToken(lastTime, lastID)
			break
		}
		task, err := decodeTask(payload)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
		lastTime = updatedAt
		lastID = a2a.TaskID(id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("taskstore: iterate tasks: %w", err)
	}

	trimmed := make([]*a2a.Task, 0, len(tasks))
	for _, task := range tasks {
		trimmed = append(trimmed, trimTask(task, req))
	}

	return &a2a.ListTasksResponse{
		Tasks:         trimmed,
		TotalSize:     totalSize,
		PageSize:      pageSize,
		NextPageToken: nextPageToken,
	}, nil
}

func (s *Postgres) authenticator(ctx context.Context) (string, error) {
	user, err := s.auth(ctx)
	if err != nil {
		return "", fmt.Errorf("taskstore auth failed: %w", err)
	}
	return user, nil
}

func (s *Postgres) existsForUser(ctx context.Context, taskID a2a.TaskID, user string) (bool, error) {
	var one int
	err := s.pool.QueryRow(ctx, `SELECT 1 FROM a2a_tasks WHERE id = $1 AND user_id = $2`, string(taskID), user).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("taskstore: ownership check: %w", err)
	}
	return true, nil
}

func decodeTask(payload []byte) (*a2a.Task, error) {
	task := &a2a.Task{}
	if err := json.Unmarshal(payload, task); err != nil {
		return nil, fmt.Errorf("taskstore: decode task: %w", err)
	}
	return task, nil
}

func statusTimestamp(task *a2a.Task) *time.Time {
	if task.Status.Timestamp == nil {
		return nil
	}
	t := *task.Status.Timestamp
	return &t
}

func trimTask(task *a2a.Task, req *a2a.ListTasksRequest) *a2a.Task {
	historyLength := defaultMaxHistoryLength
	if req.HistoryLength != nil {
		historyLength = *req.HistoryLength
	}
	switch {
	case historyLength <= 0:
		task.History = []*a2a.Message{}
	case len(task.History) > historyLength:
		task.History = task.History[len(task.History)-historyLength:]
	}
	if !req.IncludeArtifacts {
		task.Artifacts = nil
	}
	return task
}

func encodePageToken(updatedTime time.Time, taskID a2a.TaskID) string {
	timeStrNano := updatedTime.Format(time.RFC3339Nano)
	return base64.URLEncoding.EncodeToString(fmt.Appendf(nil, "%s_%s", timeStrNano, taskID))
}

func decodePageToken(nextPageToken string) (time.Time, a2a.TaskID, error) {
	decoded, err := base64.URLEncoding.DecodeString(nextPageToken)
	if err != nil {
		return time.Time{}, "", a2a.ErrParseError
	}
	parts := strings.SplitN(string(decoded), "_", 2)
	if len(parts) != 2 {
		return time.Time{}, "", a2a.ErrParseError
	}
	taskID := a2a.TaskID(parts[1])
	updatedTime, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", a2a.ErrParseError
	}
	return updatedTime, taskID, nil
}
