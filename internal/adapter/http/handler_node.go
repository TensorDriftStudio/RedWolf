package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/service"
)

// NodeHandler exposes REST endpoints for node queries and telemetry ingestion.
type NodeHandler struct {
	prov *service.Provisioner
}

// NewNodeHandler creates an initialized NodeHandler.
func NewNodeHandler(prov *service.Provisioner) *NodeHandler {
	return &NodeHandler{prov: prov}
}

// List handles GET /api/nodes.
func (h *NodeHandler) List(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.prov.ListNodes(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to query nodes list: "+err.Error())
		return
	}
	if nodes == nil {
		nodes = []*domain.ServerNode{}
	}
	writeJSON(w, http.StatusOK, nodes)
}

// Get handles GET /api/nodes/{id}.
func (h *NodeHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	node, err := h.prov.GetNode(r.Context(), id)
	if err != nil {
		if domain.ErrNodeNotFoundIs(err) {
			writeJSONError(w, http.StatusNotFound, "node not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, node)
}

// IngestTelemetry handles POST /api/nodes/telemetry from the discovery agent.
func (h *NodeHandler) IngestTelemetry(w http.ResponseWriter, r *http.Request) {
	var node domain.ServerNode
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid telemetry payload: "+err.Error())
		return
	}

	registered, err := h.prov.RegisterDiscoveredNode(r.Context(), &node)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to process telemetry: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, registered)
}

// Reset handles POST /api/nodes/{id}/reset.
func (h *NodeHandler) Reset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	node, err := h.prov.ResetNode(r.Context(), id)
	if err != nil {
		if domain.ErrNodeNotFoundIs(err) {
			writeJSONError(w, http.StatusNotFound, "node not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, node)
}

// Delete handles DELETE /api/nodes/{id}.
func (h *NodeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.prov.DeleteNode(r.Context(), id); err != nil {
		if domain.ErrNodeNotFoundIs(err) {
			writeJSONError(w, http.StatusNotFound, "node not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
