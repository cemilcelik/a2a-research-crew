// Package cli, a2a-research-crew backend uçlarını tüketen komut satırı
// istemcisini sağlar: JWT login/refresh, streaming mesaj gönderme, görev
// listeleme/inceleme, artefakt görüntüleme ve iptal.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Tokens, diske saklanan erişim/refresh token çiftidir.
type Tokens struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

// RefreshMargin, access token'ın son kullanımından önce yenilenmesi için
// bırakılan paydır.
const refreshMargin = 30 * time.Second

// TokenStore, token'ları bir dosyada saklar.
type TokenStore struct {
	path string
}

// NewTokenStore, verilen yolda çalışan bir TokenStore oluşturur.
func NewTokenStore(path string) *TokenStore {
	return &TokenStore{path: path}
}

// Path, token dosyasının yolunu döner.
func (s *TokenStore) Path() string { return s.path }

// Load, kayıtlı token'ları okur. Dosya yoksa (nil, nil) döner.
func (s *TokenStore) Load() (*Tokens, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("cli: read token file: %w", err)
	}
	var tokens Tokens
	if err := json.Unmarshal(data, &tokens); err != nil {
		return nil, fmt.Errorf("cli: decode token file: %w", err)
	}
	return &tokens, nil
}

// Save, token'ları diske yazar (yalnızca sahibinin okuyabildiği izinle).
func (s *TokenStore) Save(tokens *Tokens) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("cli: create token dir: %w", err)
	}
	data, err := json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		return fmt.Errorf("cli: encode tokens: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return fmt.Errorf("cli: write token file: %w", err)
	}
	return nil
}

// Clear, kayıtlı token'ları siler.
func (s *TokenStore) Clear() error {
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cli: remove token file: %w", err)
	}
	return nil
}

// ExpiringSoon, access token'ın yenileme payına girdiğini bildirir.
func (t *Tokens) ExpiringSoon() bool {
	return time.Until(t.ExpiresAt) < refreshMargin
}
