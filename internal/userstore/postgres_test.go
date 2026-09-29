package userstore

import (
	"context"
	"errors"
	"os"
	"testing"
)

func newTestStore(t *testing.T) *Postgres {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set; skipping PostgreSQL integration test")
	}
	store, err := NewPostgres(context.Background(), dsn)
	if err != nil {
		t.Fatalf("NewPostgres() error: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() error: %v", err)
	}
	if _, err := store.pool.Exec(context.Background(), "DELETE FROM users WHERE username = $1", "alice"); err != nil {
		t.Fatalf("cleanup error: %v", err)
	}
	t.Cleanup(store.Close)
	return store
}

func TestUserStoreUpsertAndAuthenticate(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.Upsert(ctx, "alice", "s3cret-password", []string{"researcher"}); err != nil {
		t.Fatalf("Upsert() error: %v", err)
	}

	roles, err := store.Authenticate(ctx, "alice", "s3cret-password")
	if err != nil {
		t.Fatalf("Authenticate() error: %v", err)
	}
	if len(roles) != 1 || roles[0] != "researcher" {
		t.Fatalf("roles = %v, want [researcher]", roles)
	}

	if _, err := store.Authenticate(ctx, "alice", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password error = %v, want ErrInvalidCredentials", err)
	}
	if _, err := store.Authenticate(ctx, "nobody", "whatever"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("unknown user error = %v, want ErrInvalidCredentials", err)
	}
}
