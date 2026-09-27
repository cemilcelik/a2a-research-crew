package taskstore

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	sdkstore "github.com/a2aproject/a2a-go/v2/a2asrv/taskstore"
)

// userContextKey, testlerde kimliği taşımak için kullanılan bağlam anahtarıdır.
type userContextKey struct{}

func ctxWithUser(ctx context.Context, user string) context.Context {
	return context.WithValue(ctx, userContextKey{}, user)
}

func userFromContext(ctx context.Context) (string, error) {
	if user, ok := ctx.Value(userContextKey{}).(string); ok {
		return user, nil
	}
	return "", nil
}

func newTestStore(t *testing.T) *Postgres {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set; skipping PostgreSQL integration test")
	}
	store, err := NewPostgres(context.Background(), dsn, userFromContext)
	if err != nil {
		t.Fatalf("NewPostgres() error: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() error: %v", err)
	}
	if _, err := store.pool.Exec(context.Background(), "TRUNCATE a2a_tasks"); err != nil {
		t.Fatalf("truncate error: %v", err)
	}
	t.Cleanup(store.Close)
	return store
}

func sampleTask(id, contextID string, state a2a.TaskState) *a2a.Task {
	return &a2a.Task{
		ID:        a2a.TaskID(id),
		ContextID: contextID,
		Status:    a2a.TaskStatus{State: state},
		History: []*a2a.Message{
			a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("hello")),
		},
	}
}

func TestPostgresCreateAndGet(t *testing.T) {
	store := newTestStore(t)
	ctx := ctxWithUser(context.Background(), "alice")

	version, err := store.Create(ctx, sampleTask("t1", "c1", a2a.TaskStateSubmitted))
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if version != sdkstore.TaskVersion(1) {
		t.Errorf("version = %d, want 1", version)
	}

	stored, err := store.Get(ctx, "t1")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if stored.Task.ContextID != "c1" {
		t.Errorf("ContextID = %q, want c1", stored.Task.ContextID)
	}
	if stored.User != "alice" {
		t.Errorf("User = %q, want alice", stored.User)
	}
}

func TestPostgresCreateDuplicate(t *testing.T) {
	store := newTestStore(t)
	ctx := ctxWithUser(context.Background(), "alice")
	task := sampleTask("t1", "c1", a2a.TaskStateSubmitted)

	if _, err := store.Create(ctx, task); err != nil {
		t.Fatalf("first Create() error: %v", err)
	}
	if _, err := store.Create(ctx, task); !errors.Is(err, sdkstore.ErrTaskAlreadyExists) {
		t.Fatalf("second Create() error = %v, want ErrTaskAlreadyExists", err)
	}
}

func TestPostgresOwnershipIsolation(t *testing.T) {
	store := newTestStore(t)
	task := sampleTask("t1", "c1", a2a.TaskStateSubmitted)
	if _, err := store.Create(ctxWithUser(context.Background(), "alice"), task); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if _, err := store.Get(ctxWithUser(context.Background(), "bob"), "t1"); !errors.Is(err, a2a.ErrTaskNotFound) {
		t.Fatalf("Get() as other user error = %v, want ErrTaskNotFound", err)
	}
}

func TestPostgresUpdateOCC(t *testing.T) {
	store := newTestStore(t)
	ctx := ctxWithUser(context.Background(), "alice")
	task := sampleTask("t1", "c1", a2a.TaskStateSubmitted)
	version, err := store.Create(ctx, task)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	updated := sampleTask("t1", "c1", a2a.TaskStateWorking)
	newVersion, err := store.Update(ctx, &sdkstore.UpdateRequest{
		Task:        updated,
		PrevVersion: version,
	})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if newVersion <= version {
		t.Errorf("newVersion = %d, want > %d", newVersion, version)
	}

	// Bayat sürümle güncelleme çakışma döndürmelidir.
	_, err = store.Update(ctx, &sdkstore.UpdateRequest{
		Task:        sampleTask("t1", "c1", a2a.TaskStateCompleted),
		PrevVersion: version,
	})
	if !errors.Is(err, sdkstore.ErrConcurrentModification) {
		t.Fatalf("stale Update() error = %v, want ErrConcurrentModification", err)
	}
}

func TestPostgresUpdateMissing(t *testing.T) {
	store := newTestStore(t)
	ctx := ctxWithUser(context.Background(), "alice")

	_, err := store.Update(ctx, &sdkstore.UpdateRequest{
		Task: sampleTask("missing", "c1", a2a.TaskStateWorking),
	})
	if !errors.Is(err, a2a.ErrTaskNotFound) {
		t.Fatalf("Update() error = %v, want ErrTaskNotFound", err)
	}
}

func TestPostgresList(t *testing.T) {
	store := newTestStore(t)
	ctx := ctxWithUser(context.Background(), "alice")

	for i := 0; i < 3; i++ {
		task := sampleTask(taskID(i), "c1", a2a.TaskStateSubmitted)
		if _, err := store.Create(ctx, task); err != nil {
			t.Fatalf("Create(%d) error: %v", i, err)
		}
	}
	if _, err := store.Create(ctx, sampleTask("other", "c2", a2a.TaskStateSubmitted)); err != nil {
		t.Fatalf("Create(other) error: %v", err)
	}

	res, err := store.List(ctx, &a2a.ListTasksRequest{ContextID: "c1", PageSize: 2})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if res.TotalSize != 3 {
		t.Errorf("TotalSize = %d, want 3", res.TotalSize)
	}
	if len(res.Tasks) != 2 {
		t.Fatalf("len(Tasks) = %d, want 2", len(res.Tasks))
	}
	if res.NextPageToken == "" {
		t.Error("NextPageToken = empty, want a cursor")
	}

	res2, err := store.List(ctx, &a2a.ListTasksRequest{ContextID: "c1", PageSize: 2, PageToken: res.NextPageToken})
	if err != nil {
		t.Fatalf("second List() error: %v", err)
	}
	if len(res2.Tasks) != 1 {
		t.Errorf("second page len = %d, want 1", len(res2.Tasks))
	}
}

func TestPostgresListRequiresUser(t *testing.T) {
	store := newTestStore(t)
	_, err := store.List(context.Background(), &a2a.ListTasksRequest{})
	if !errors.Is(err, a2a.ErrUnauthenticated) {
		t.Fatalf("List() error = %v, want ErrUnauthenticated", err)
	}
}

func taskID(i int) string {
	return "t" + string(rune('0'+i))
}
