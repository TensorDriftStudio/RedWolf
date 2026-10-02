package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/service"
)

// DeployHandler manages provisioning triggers and live progress updates.
type DeployHandler struct {
	prov *service.Provisioner
}

// NewDeployHandler creates an initialized DeployHandler.
func NewDeployHandler(prov *service.Provisioner) *DeployHandler {
	return &DeployHandler{prov: prov}
}

// Deploy triggers OS provisioning on a ready node.
func (h *DeployHandler) Deploy(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "id")

	var cfg domain.DeploymentConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid deployment config: "+err.Error())
		return
	}
	cfg.NodeID = nodeID

	if err := h.prov.InitiateDeployment(r.Context(), cfg); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  "provisioning_initiated",
		"node_id": nodeID,
	})
}

// ProgressRequest represents progress callbacks from the provisioning agent.
type ProgressRequest struct {
	Progress int    `json:"progress"`
	Stage    string `json:"stage"`
	Log      string `json:"log"`
}

// UpdateProgress handles POST /api/nodes/{id}/progress from the agent.
func (h *DeployHandler) UpdateProgress(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "id")

	var req ProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid progress body: "+err.Error())
		return
	}

	if err := h.prov.UpdateProgress(r.Context(), nodeID, req.Progress, req.Stage, req.Log); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GetTask handles GET /api/nodes/{id}/task for agent polling.
func (h *DeployHandler) GetTask(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "id")
	task, err := h.prov.GetPendingTask(r.Context(), nodeID)
	if err != nil {
		if domain.ErrNodeNotFoundIs(err) {
			writeJSONError(w, http.StatusNotFound, "no active deployment task")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, task)
}

// GetTaskByMAC handles GET /api/nodes/task/by-mac/{mac} for agent polling by physical MAC.
func (h *DeployHandler) GetTaskByMAC(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")
	task, err := h.prov.GetPendingTaskByMAC(r.Context(), mac)
	if err != nil {
		if domain.ErrNodeNotFoundIs(err) {
			writeJSONError(w, http.StatusNotFound, "no active deployment task for mac: "+mac)
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, task)
}
