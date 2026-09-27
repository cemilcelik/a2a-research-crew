package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

func testManager(t *testing.T) *Manager {
	t.Helper()
	m, err := NewManager(Config{
		Secret:     []byte("0123456789abcdef0123456789abcdef"),
		Issuer:     "test-issuer",
		AccessTTL:  time.Minute,
		RefreshTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewManager() error: %v", err)
	}
	return m
}

func TestNewManagerRejectsWeakSecret(t *testing.T) {
	_, err := NewManager(Config{Secret: []byte("short"), Issuer: "x", AccessTTL: time.Minute, RefreshTTL: time.Hour})
	if !errors.Is(err, ErrWeakSecret) {
		t.Fatalf("NewManager() error = %v, want ErrWeakSecret", err)
	}
}

func TestIssueAndParseAccess(t *testing.T) {
	m := testManager(t)
	pair, err := m.Issue("alice", []string{"researcher"})
	if err != nil {
		t.Fatalf("Issue() error: %v", err)
	}
	if pair.TokenType != "Bearer" || pair.ExpiresIn != 60 {
		t.Errorf("pair = %+v, want Bearer/60", pair)
	}

	claims, err := m.ParseAccess(pair.AccessToken)
	if err != nil {
		t.Fatalf("ParseAccess() error: %v", err)
	}
	if claims.Subject != "alice" {
		t.Errorf("Subject = %q, want alice", claims.Subject)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "researcher" {
		t.Errorf("Roles = %v, want [researcher]", claims.Roles)
	}
}

func TestParseAccessRejectsRefreshToken(t *testing.T) {
	m := testManager(t)
	pair, _ := m.Issue("alice", nil)
	if _, err := m.ParseAccess(pair.RefreshToken); !errors.Is(err, ErrWrongTokenType) {
		t.Fatalf("ParseAccess(refresh) error = %v, want ErrWrongTokenType", err)
	}
}

func TestRefresh(t *testing.T) {
	m := testManager(t)
	pair, _ := m.Issue("bob", []string{"writer"})

	refreshed, err := m.Refresh(pair.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh() error: %v", err)
	}
	claims, err := m.ParseAccess(refreshed.AccessToken)
	if err != nil {
		t.Fatalf("ParseAccess() error: %v", err)
	}
	if claims.Subject != "bob" {
		t.Errorf("Subject = %q, want bob", claims.Subject)
	}
}

func TestParseRejectsExpiredToken(t *testing.T) {
	m, err := NewManager(Config{
		Secret:     []byte("0123456789abcdef0123456789abcdef"),
		Issuer:     "test-issuer",
		AccessTTL:  time.Millisecond,
		RefreshTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewManager() error: %v", err)
	}
	pair, _ := m.Issue("carol", nil)
	time.Sleep(5 * time.Millisecond)

	if _, err := m.ParseAccess(pair.AccessToken); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("ParseAccess() error = %v, want ErrExpiredToken", err)
	}
}

func TestParseRejectsTamperedToken(t *testing.T) {
	m := testManager(t)
	pair, _ := m.Issue("dave", nil)

	if _, err := m.ParseAccess(pair.AccessToken + "x"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("ParseAccess() error = %v, want ErrInvalidToken", err)
	}
}

func TestServerInterceptorRequired(t *testing.T) {
	m := testManager(t)
	interceptor := NewServerInterceptor(m, true)

	_, callCtx := a2asrv.NewCallContext(context.Background(), a2asrv.NewServiceParams(nil))
	if _, _, err := interceptor.Before(context.Background(), callCtx, nil); err == nil {
		t.Fatal("Before() = nil error, want unauthenticated error for missing token")
	}
}

func TestServerInterceptorSetsUser(t *testing.T) {
	m := testManager(t)
	pair, _ := m.Issue("erin", []string{"analyst"})
	interceptor := NewServerInterceptor(m, true)

	params := a2asrv.NewServiceParams(map[string][]string{
		ServiceParamAuthorization: {BearerPrefix + pair.AccessToken},
	})
	_, callCtx := a2asrv.NewCallContext(context.Background(), params)
	if _, _, err := interceptor.Before(context.Background(), callCtx, nil); err != nil {
		t.Fatalf("Before() error: %v", err)
	}
	if callCtx.User == nil || !callCtx.User.Authenticated || callCtx.User.Name != "erin" {
		t.Fatalf("User = %+v, want authenticated erin", callCtx.User)
	}
}

func TestClientInterceptorAddsHeader(t *testing.T) {
	interceptor := NewClientInterceptor(StaticTokenSource("tok123"))
	req := &a2aclient.Request{}
	if _, _, err := interceptor.Before(context.Background(), req); err != nil {
		t.Fatalf("Before() error: %v", err)
	}
	got := req.ServiceParams.Get(ServiceParamAuthorization)
	if len(got) != 1 || got[0] != "Bearer tok123" {
		t.Fatalf("authorization = %v, want [Bearer tok123]", got)
	}
}
