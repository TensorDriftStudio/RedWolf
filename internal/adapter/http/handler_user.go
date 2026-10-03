package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/service"
)

// UserHandler manages REST endpoints for operator identity administration.
type UserHandler struct {
	userSvc *service.UserService
	authSvc *service.AuthService
}

// NewUserHandler creates an initialized UserHandler.
func NewUserHandler(userSvc *service.UserService, authSvc *service.AuthService) *UserHandler {
	return &UserHandler{
		userSvc: userSvc,
		authSvc: authSvc,
	}
}

// List handles GET /api/users.
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	users, err := h.userSvc.ListUsers(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed listing users: "+err.Error())
		return
	}
	if users == nil {
		users = []*domain.User{}
	}
	writeJSON(w, http.StatusOK, users)
}

// Get handles GET /api/users/{id}.
func (h *UserHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	user, err := h.userSvc.GetUserByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			writeJSONError(w, http.StatusNotFound, "user not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed getting user: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// Create handles POST /api/users.
func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	user, err := h.userSvc.CreateUser(r.Context(), req)
	if err != nil {
		if errors.Is(err, domain.ErrUserExists) {
			writeJSONError(w, http.StatusConflict, "username already exists")
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, user)
}

// Update handles PUT /api/users/{id}.
func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req domain.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	user, err := h.userSvc.UpdateUser(r.Context(), id, req)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			writeJSONError(w, http.StatusNotFound, "user not found")
			return
		}
		if errors.Is(err, domain.ErrCannotDeleteLastAdmin) {
			writeJSONError(w, http.StatusForbidden, "cannot remove administrator role from the last active admin")
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, user)
}

// ChangePassword handles POST /api/users/{id}/password.
func (h *UserHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req domain.ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if err := h.userSvc.ChangePassword(r.Context(), id, req.NewPassword); err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			writeJSONError(w, http.StatusNotFound, "user not found")
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

// Delete handles DELETE /api/users/{id}.
func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	callerID := ""
	if user := UserFromContext(r.Context()); user != nil {
		callerID = user.ID
	} else {
		token := extractBearerToken(r)
		if token != "" && h.authSvc != nil {
			if caller, ok := h.authSvc.ValidateToken(token); ok && caller != nil {
				callerID = caller.ID
			}
		}
	}

	if err := h.userSvc.DeleteUser(r.Context(), id, callerID); err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			writeJSONError(w, http.StatusNotFound, "user not found")
			return
		}
		if errors.Is(err, domain.ErrCannotDeleteSelf) {
			writeJSONError(w, http.StatusForbidden, "cannot delete your own active user account")
			return
		}
		if errors.Is(err, domain.ErrCannotDeleteLastAdmin) {
			writeJSONError(w, http.StatusForbidden, "cannot delete the last remaining administrator account")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed deleting user: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
