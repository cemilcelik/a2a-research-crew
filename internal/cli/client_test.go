package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	return &App{
		Out:        io.Discard,
		Err:        io.Discard,
		Tokens:     NewTokenStore(t.TempDir() + "/tokens.json"),
		HTTPClient: http.DefaultClient,
	}
}

func TestLoginRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/login" {
			t.Errorf("path = %q, want /auth/login", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"accessToken":"a","refreshToken":"r","tokenType":"Bearer","expiresIn":120}`))
	}))
	defer server.Close()

	app := newTestApp(t)
	tokens, err := app.loginRequest(context.Background(), server.URL, "alice", "secret")
	if err != nil {
		t.Fatalf("loginRequest() error: %v", err)
	}
	if tokens.AccessToken != "a" || tokens.RefreshToken != "r" {
		t.Fatalf("tokens = %+v, want a/r", tokens)
	}
	if tokens.ExpiringSoon() {
		t.Error("fresh token should not be expiring soon")
	}
}

func TestLoginRequestRejects401(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid credentials"}`))
	}))
	defer server.Close()

	app := newTestApp(t)
	if _, err := app.loginRequest(context.Background(), server.URL, "alice", "wrong"); err == nil {
		t.Fatal("loginRequest() = nil error, want 401 error")
	}
}

func TestEnsureAccessTokenRefreshes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/refresh" {
			t.Errorf("path = %q, want /auth/refresh", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"accessToken":"new-access","refreshToken":"r2","expiresIn":120}`))
	}))
	defer server.Close()

	// Kart çözümlemesini taklit etmek yerine restBaseURL'yi doğrudan test ederiz.
	restURL, err := restBaseURL(&a2a.AgentCard{
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(server.URL, a2a.TransportProtocolHTTPJSON),
		},
	})
	if err != nil {
		t.Fatalf("restBaseURL() error: %v", err)
	}
	if restURL != server.URL {
		t.Fatalf("restBaseURL() = %q, want %q", restURL, server.URL)
	}

	app := newTestApp(t)
	tokens, err := app.refreshRequest(context.Background(), restURL, "old-refresh")
	if err != nil {
		t.Fatalf("refreshRequest() error: %v", err)
	}
	if tokens.AccessToken != "new-access" {
		t.Fatalf("tokens = %+v, want new-access", tokens)
	}
}

func TestRestBaseURLMissingInterface(t *testing.T) {
	_, err := restBaseURL(&a2a.AgentCard{
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface("127.0.0.1:9000", a2a.TransportProtocolGRPC),
		},
	})
	if err == nil || !strings.Contains(err.Error(), "HTTP+JSON") {
		t.Fatalf("restBaseURL() error = %v, want HTTP+JSON error", err)
	}
}
