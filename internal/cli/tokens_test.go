package cli

import (
	"path/filepath"
	"testing"
	"time"
)

func TestTokenStoreRoundTrip(t *testing.T) {
	store := NewTokenStore(filepath.Join(t.TempDir(), "tokens.json"))

	if tokens, err := store.Load(); err != nil || tokens != nil {
		t.Fatalf("Load() = %v, %v; want nil, nil", tokens, err)
	}

	want := &Tokens{
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(time.Hour).Truncate(time.Second),
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got.AccessToken != "access" || got.RefreshToken != "refresh" {
		t.Fatalf("Load() = %+v, want access/refresh", got)
	}

	if err := store.Clear(); err != nil {
		t.Fatalf("Clear() error: %v", err)
	}
	if tokens, _ := store.Load(); tokens != nil {
		t.Fatalf("Load() after Clear() = %+v, want nil", tokens)
	}
}

func TestTokensExpiringSoon(t *testing.T) {
	if !(&Tokens{ExpiresAt: time.Now().Add(time.Second)}).ExpiringSoon() {
		t.Error("token expiring in 1s should be ExpiringSoon")
	}
	if (&Tokens{ExpiresAt: time.Now().Add(time.Hour)}).ExpiringSoon() {
		t.Error("token expiring in 1h should not be ExpiringSoon")
	}
}
