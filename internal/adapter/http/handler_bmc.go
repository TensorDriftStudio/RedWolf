package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/service"
)

// BMCHandler manages BMC credential escrow and remote power endpoints.
type BMCHandler struct {
	escrowSvc *service.BMCEscrowService
	bmcMgr    *service.BMCManager
}

// NewBMCHandler creates an initialized BMCHandler.
func NewBMCHandler(escrowSvc *service.BMCEscrowService, bmcMgr *service.BMCManager) *BMCHandler {
	return &BMCHandler{
		escrowSvc: escrowSvc,
		bmcMgr:    bmcMgr,
	}
}

// Rotate handles POST /api/nodes/{id}/bmc/rotate.
func (h *BMCHandler) Rotate(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "id")

	type rotateReq struct {
		Username string `json:"username,omitempty"`
		UserSlot int    `json:"userSlot,omitempty"`
	}
	var req rotateReq
	_ = json.NewDecoder(r.Body).Decode(&req)

	cred, err := h.escrowSvc.RotateCredentials(r.Context(), nodeID, req.Username, req.UserSlot)
	if err != nil {
		if domain.ErrNodeNotFoundIs(err) {
			writeJSONError(w, http.StatusNotFound, "node not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed rotating bmc credentials: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, cred)
}

// GetCredentials handles GET /api/nodes/{id}/bmc/credentials.
func (h *BMCHandler) GetCredentials(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "id")

	cred, err := h.escrowSvc.GetCredentials(r.Context(), nodeID)
	if err != nil {
		if domain.ErrNodeNotFoundIs(err) {
			writeJSONError(w, http.StatusNotFound, "no escrowed credentials found for this node")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed retrieving credentials: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, cred)
}

// GetPower handles GET /api/nodes/{id}/power.
func (h *BMCHandler) GetPower(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "id")
	if h.bmcMgr == nil {
		writeJSONError(w, http.StatusNotImplemented, "bmc manager not configured")
		return
	}
	resp, err := h.bmcMgr.GetNodePowerState(r.Context(), nodeID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// SetPower handles POST /api/nodes/{id}/power.
func (h *BMCHandler) SetPower(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "id")
	if h.bmcMgr == nil {
		writeJSONError(w, http.StatusNotImplemented, "bmc manager not configured")
		return
	}

	var req struct {
		Action domain.PowerAction `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid power action body: "+err.Error())
		return
	}

	if err := h.bmcMgr.ExecuteNodePowerAction(r.Context(), nodeID, req.Action); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"action":  string(req.Action),
		"node_id": nodeID,
	})
}
