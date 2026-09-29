// Package authhttp, orchestrator'ın JWT tabanlı /auth/login ve /auth/refresh
// uçlarını sağlar.
package authhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"a2a-research-crew/internal/auth"
	"a2a-research-crew/internal/userstore"
)

// UserAuthenticator, parola doğrulamasını soyutlar.
type UserAuthenticator interface {
	// Authenticate, kimlik bilgilerini doğrular ve rolleri döner.
	Authenticate(ctx context.Context, username, password string) ([]string, error)
}

// Handler, kimlik doğrulama uçlarını taşır.
type Handler struct {
	manager *auth.Manager
	users   UserAuthenticator
}

// NewHandler, verilen JWT yöneticisi ve kullanıcı deposu ile bir Handler oluşturur.
func NewHandler(manager *auth.Manager, users UserAuthenticator) *Handler {
	return &Handler{manager: manager, users: users}
}

// Register, uçları standart mux'a kaydeder.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/login", h.login)
	mux.HandleFunc("POST /auth/refresh", h.refresh)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	roles, err := h.users.Authenticate(r.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, userstore.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeError(w, http.StatusInternalServerError, "authentication failed")
		return
	}

	pair, err := h.manager.Issue(req.Username, roles)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	writeJSON(w, http.StatusOK, pair)
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refreshToken is required")
		return
	}

	pair, err := h.manager.Refresh(req.RefreshToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	writeJSON(w, http.StatusOK, pair)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}
