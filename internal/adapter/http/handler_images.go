package http

import (
	"encoding/json"
	"net/http"

	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/service"
)

// ImageHandler handles HTTP endpoints for the OS image catalog.
type ImageHandler struct {
	catalog *service.ImageCatalogService
}

// NewImageHandler creates an initialized ImageHandler.
func NewImageHandler(catalog *service.ImageCatalogService) *ImageHandler {
	return &ImageHandler{catalog: catalog}
}

// List handles GET /api/images.
func (h *ImageHandler) List(w http.ResponseWriter, r *http.Request) {
	images, err := h.catalog.ListImages(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed listing OS images: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, images)
}

// Download handles POST /api/images/download.
func (h *ImageHandler) Download(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OS domain.OperatingSystem `json:"os"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid image download request: "+err.Error())
		return
	}

	status, err := h.catalog.DownloadImage(r.Context(), req.OS)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, status)
}

// Status handles GET /api/images/status.
func (h *ImageHandler) Status(w http.ResponseWriter, r *http.Request) {
	statuses := h.catalog.GetDownloadStatuses()
	writeJSON(w, http.StatusOK, statuses)
}
