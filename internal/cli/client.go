package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"

	"a2a-research-crew/internal/auth"
)

// App, CLI komutlarının paylaştığı bağımlılıkları taşır.
type App struct {
	Out        io.Writer
	Err        io.Writer
	ServerURL  string
	Tokens     *TokenStore
	HTTPClient *http.Client
}

// New, bir App oluşturur.
func New(serverURL, tokenPath string, out, errOut io.Writer) *App {
	return &App{
		Out:        out,
		Err:        errOut,
		ServerURL:  strings.TrimRight(serverURL, "/"),
		Tokens:     NewTokenStore(tokenPath),
		HTTPClient: &http.Client{Timeout: 5 * time.Minute},
	}
}

// resolve, orchestrator'ın public AgentCard'ını çeker.
func (a *App) resolve(ctx context.Context) (*a2a.AgentCard, error) {
	card, err := agentcard.DefaultResolver.Resolve(ctx, a.ServerURL)
	if err != nil {
		return nil, fmt.Errorf("could not resolve agent card at %s: %w", a.ServerURL, err)
	}
	return card, nil
}

// restBaseURL, karttaki HTTP+JSON/REST arayüzünün taban URL'sini döner.
func restBaseURL(card *a2a.AgentCard) (string, error) {
	for _, iface := range card.SupportedInterfaces {
		if iface.ProtocolBinding == a2a.TransportProtocolHTTPJSON {
			return strings.TrimRight(iface.URL, "/"), nil
		}
	}
	return "", fmt.Errorf("agent does not expose an HTTP+JSON interface")
}

// authenticatedClient, geçerli bir token ile REST tabanlı bir A2A istemcisi
// oluşturur.
func (a *App) authenticatedClient(ctx context.Context) (*a2aclient.Client, error) {
	token, err := a.ensureAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	card, err := a.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return a2aclient.NewFromCard(ctx, card,
		a2aclient.WithRESTTransport(a.HTTPClient),
		a2aclient.WithCallInterceptors(auth.NewClientInterceptor(auth.StaticTokenSource(token))),
	)
}

// ensureAccessToken, kayıtlı token'ı döner; gerekirse yeniler.
func (a *App) ensureAccessToken(ctx context.Context) (string, error) {
	tokens, err := a.Tokens.Load()
	if err != nil {
		return "", err
	}
	if tokens == nil {
		return "", fmt.Errorf("not logged in; run `login` first")
	}
	if !tokens.ExpiringSoon() {
		return tokens.AccessToken, nil
	}
	if tokens.RefreshToken == "" {
		return "", fmt.Errorf("access token expired and no refresh token available; run `login` again")
	}
	base, err := a.restBase(ctx)
	if err != nil {
		return "", err
	}
	refreshed, err := a.refreshRequest(ctx, base, tokens.RefreshToken)
	if err != nil {
		return "", err
	}
	if err := a.Tokens.Save(refreshed); err != nil {
		return "", err
	}
	return refreshed.AccessToken, nil
}

func (a *App) restBase(ctx context.Context) (string, error) {
	card, err := a.resolve(ctx)
	if err != nil {
		return "", err
	}
	return restBaseURL(card)
}

type authResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	TokenType    string `json:"tokenType"`
	ExpiresIn    int    `json:"expiresIn"`
}

func (a *App) loginRequest(ctx context.Context, base, username, password string) (*Tokens, error) {
	payload, _ := json.Marshal(map[string]string{"username": username, "password": password})
	return a.postAuth(ctx, base+"/auth/login", payload)
}

func (a *App) refreshRequest(ctx context.Context, base, refreshToken string) (*Tokens, error) {
	payload, _ := json.Marshal(map[string]string{"refreshToken": refreshToken})
	return a.postAuth(ctx, base+"/auth/refresh", payload)
}

func (a *App) postAuth(ctx context.Context, url string, payload []byte) (*Tokens, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("cli: build auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cli: auth request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("authentication failed (status %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed authResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("cli: decode auth response: %w", err)
	}
	expiresIn := parsed.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 900
	}
	return &Tokens{
		AccessToken:  parsed.AccessToken,
		RefreshToken: parsed.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second),
	}, nil
}
