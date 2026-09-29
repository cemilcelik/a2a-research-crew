package authhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"a2a-research-crew/internal/auth"
	"a2a-research-crew/internal/userstore"
)

type fakeUsers struct {
	passwords map[string]string
}

func (f fakeUsers) Authenticate(_ context.Context, username, password string) ([]string, error) {
	if password == f.passwords[username] && password != "" {
		return []string{"researcher"}, nil
	}
	return nil, userstore.ErrInvalidCredentials
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	manager, err := auth.NewManager(auth.Config{
		Secret:     []byte("0123456789abcdef0123456789abcdef"),
		Issuer:     "test",
		AccessTTL:  time.Minute,
		RefreshTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewManager() error: %v", err)
	}
	handler := NewHandler(manager, fakeUsers{passwords: map[string]string{"alice": "s3cret"}})
	mux := http.NewServeMux()
	handler.Register(mux)
	return httptest.NewServer(mux)
}

func postJSON(t *testing.T, url string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("POST %s error: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var decoded map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&decoded)
	return resp.StatusCode, decoded
}

func TestLoginAndRefresh(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	status, body := postJSON(t, server.URL+"/auth/login", map[string]string{
		"username": "alice", "password": "s3cret",
	})
	if status != http.StatusOK {
		t.Fatalf("login status = %d, want 200 (%v)", status, body)
	}
	if body["accessToken"] == "" || body["refreshToken"] == "" {
		t.Fatalf("login body = %v, want tokens", body)
	}

	refreshToken, _ := body["refreshToken"].(string)
	status, refreshed := postJSON(t, server.URL+"/auth/refresh", map[string]string{"refreshToken": refreshToken})
	if status != http.StatusOK || refreshed["accessToken"] == "" {
		t.Fatalf("refresh status = %d body = %v, want new tokens", status, refreshed)
	}
}

func TestLoginRejectsBadCredentials(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	status, _ := postJSON(t, server.URL+"/auth/login", map[string]string{
		"username": "alice", "password": "wrong",
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
}

func TestRefreshRejectsBadToken(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	status, _ := postJSON(t, server.URL+"/auth/refresh", map[string]string{"refreshToken": "nope"})
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
}
