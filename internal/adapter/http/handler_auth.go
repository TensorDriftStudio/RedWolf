package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/service"
)

// AuthHandler handles HTTP requests for login, logout, and user profile.
type AuthHandler struct {
	authSvc *service.AuthService
}

// NewAuthHandler creates an initialized AuthHandler.
func NewAuthHandler(authSvc *service.AuthService) *AuthHandler {
	return &AuthHandler{authSvc: authSvc}
}

// Login handles POST /api/auth/login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req domain.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid login request payload: "+err.Error())
		return
	}

	if req.Username == "" || req.Password == "" {
		writeJSONError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	if req.Source == "" {
		req.Source = domain.AuthSourceLocal
	}

	resp, err := h.authSvc.Login(r.Context(), req)
	if err != nil {
		if err == domain.ErrInvalidCredentials {
			writeJSONError(w, http.StatusUnauthorized, "invalid username or password")
			return
		}
		writeJSONError(w, http.StatusUnauthorized, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// Me handles GET /api/auth/me.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	if token == "" {
		writeJSONError(w, http.StatusUnauthorized, "missing authorization bearer token")
		return
	}

	user, ok := h.authSvc.ValidateToken(token)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "invalid or expired session token")
		return
	}

	writeJSON(w, http.StatusOK, user)
}

// Logout handles POST /api/auth/logout.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	if token != "" {
		h.authSvc.Logout(token)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"loggedOut": true})
}

func extractBearerToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	// Fallback to cookie
	if cookie, err := r.Cookie("redwolf_session"); err == nil {
		return cookie.Value
	}
	return ""
}
