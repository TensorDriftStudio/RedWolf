package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/service"
)

// TemplateHandler manages HTTP endpoints for Cloud-Init templates.
type TemplateHandler struct {
	templates *service.TemplateService
}

// NewTemplateHandler initializes the template HTTP handler.
func NewTemplateHandler(templates *service.TemplateService) *TemplateHandler {
	return &TemplateHandler{templates: templates}
}

// List handles GET /api/templates.
func (h *TemplateHandler) List(w http.ResponseWriter, r *http.Request) {
	templates, err := h.templates.ListTemplates(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list templates: "+err.Error())
		return
	}
	if templates == nil {
		templates = []domain.CloudInitTemplate{}
	}
	writeJSON(w, http.StatusOK, templates)
}

// Get handles GET /api/templates/{id}.
func (h *TemplateHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tpl, err := h.templates.GetTemplate(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrTemplateNotFound) {
			writeJSONError(w, http.StatusNotFound, "template not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed retrieving template: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tpl)
}

// Create handles POST /api/templates.
func (h *TemplateHandler) Create(w http.ResponseWriter, r *http.Request) {
	var tpl domain.CloudInitTemplate
	if err := json.NewDecoder(r.Body).Decode(&tpl); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request payload: "+err.Error())
		return
	}

	if err := h.templates.CreateTemplate(r.Context(), &tpl); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, tpl)
}

// Update handles PUT /api/templates/{id}.
func (h *TemplateHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var tpl domain.CloudInitTemplate
	if err := json.NewDecoder(r.Body).Decode(&tpl); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request payload: "+err.Error())
		return
	}

	if err := h.templates.UpdateTemplate(r.Context(), id, &tpl); err != nil {
		if errors.Is(err, domain.ErrTemplateNotFound) {
			writeJSONError(w, http.StatusNotFound, "template not found")
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, tpl)
}

// Delete handles DELETE /api/templates/{id}.
func (h *TemplateHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.templates.DeleteTemplate(r.Context(), id); err != nil {
		if errors.Is(err, domain.ErrTemplateNotFound) {
			writeJSONError(w, http.StatusNotFound, "template not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to delete template: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "deleted",
		"id":     id,
	})
}
