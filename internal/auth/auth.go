// Package auth, HS256 tabanlı JWT üretimi/doğrulaması ve A2A çağrıları için
// kimlik doğrulama interceptor'larını sağlar.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Token türleri.
const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

// BearerPrefix, Authorization başlığındaki şema önekidir.
const BearerPrefix = "Bearer "

// minSecretLength, HS256 için önerilen en küçük secret uzunluğudur.
const minSecretLength = 32

var (
	// ErrInvalidToken, token imzası/formatı geçersiz olduğunda döner.
	ErrInvalidToken = errors.New("auth: invalid token")
	// ErrExpiredToken, token süresi dolduğunda döner.
	ErrExpiredToken = errors.New("auth: expired token")
	// ErrWrongTokenType, beklenen türle eşleşmeyen token verildiğinde döner.
	ErrWrongTokenType = errors.New("auth: wrong token type")
	// ErrWeakSecret, secret çok kısa olduğunda döner.
	ErrWeakSecret = fmt.Errorf("auth: secret must be at least %d bytes", minSecretLength)
)

// Config, Manager yapılandırmasıdır.
type Config struct {
	Secret     []byte
	Issuer     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

// Claims, bizim ürettiğimiz JWT claim'leridir.
type Claims struct {
	TokenType string   `json:"typ"`
	Roles     []string `json:"roles,omitempty"`
	jwt.RegisteredClaims
}

// TokenPair, üretilen access ve refresh token çiftidir.
type TokenPair struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	TokenType    string `json:"tokenType"`
	ExpiresIn    int    `json:"expiresIn"`
}

// Manager, JWT üretir ve doğrular.
type Manager struct {
	cfg    Config
	parser *jwt.Parser
}

// NewManager, verilen yapılandırmayla bir Manager oluşturur.
func NewManager(cfg Config) (*Manager, error) {
	if len(cfg.Secret) < minSecretLength {
		return nil, ErrWeakSecret
	}
	if cfg.Issuer == "" {
		return nil, errors.New("auth: issuer must not be empty")
	}
	if cfg.AccessTTL <= 0 || cfg.RefreshTTL <= 0 {
		return nil, errors.New("auth: token TTLs must be positive")
	}

	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(cfg.Issuer),
		jwt.WithExpirationRequired(),
	)
	return &Manager{cfg: cfg, parser: parser}, nil
}

// Issue, verilen kullanıcı için yeni bir token çifti üretir.
func (m *Manager) Issue(subject string, roles []string) (TokenPair, error) {
	if subject == "" {
		return TokenPair{}, errors.New("auth: subject must not be empty")
	}
	access, _, err := m.sign(subject, TokenTypeAccess, roles, m.cfg.AccessTTL)
	if err != nil {
		return TokenPair{}, err
	}
	refresh, _, err := m.sign(subject, TokenTypeRefresh, roles, m.cfg.RefreshTTL)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int(m.cfg.AccessTTL.Seconds()),
	}, nil
}

// Refresh, geçerli bir refresh token ile yeni bir token çifti üretir.
func (m *Manager) Refresh(refreshToken string) (TokenPair, error) {
	claims, err := m.Parse(refreshToken, TokenTypeRefresh)
	if err != nil {
		return TokenPair{}, err
	}
	return m.Issue(claims.Subject, claims.Roles)
}

// ParseAccess, access token'ı doğrular ve claim'lerini döner.
func (m *Manager) ParseAccess(token string) (*Claims, error) {
	return m.Parse(token, TokenTypeAccess)
}

// Parse, token'ı doğrular ve beklenen türle eşleştiğini kontrol eder.
func (m *Manager) Parse(token, expectedType string) (*Claims, error) {
	claims := &Claims{}
	parsed, err := m.parser.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		return m.cfg.Secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if !parsed.Valid {
		return nil, ErrInvalidToken
	}
	if claims.TokenType != expectedType {
		return nil, fmt.Errorf("%w: got %q, want %q", ErrWrongTokenType, claims.TokenType, expectedType)
	}
	return claims, nil
}

func (m *Manager) sign(subject, tokenType string, roles []string, ttl time.Duration) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(ttl)
	claims := &Claims{
		TokenType: tokenType,
		Roles:     roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			Issuer:    m.cfg.Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.cfg.Secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: sign token: %w", err)
	}
	return signed, expiresAt, nil
}
