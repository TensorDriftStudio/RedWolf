package http

import (
	"encoding/json"
	"net/http"

	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/port"
	"github.com/tensordriftstudio/redwolf/internal/service"
)

// SettingsHandler manages HTTP endpoints for system settings and directory diagnostics.
type SettingsHandler struct {
	settingsSvc *service.SettingsService
	dnsmasq     port.DNSMasqManager
}

// NewSettingsHandler creates an initialized SettingsHandler.
func NewSettingsHandler(settingsSvc *service.SettingsService, dnsmasq port.DNSMasqManager) *SettingsHandler {
	return &SettingsHandler{
		settingsSvc: settingsSvc,
		dnsmasq:     dnsmasq,
	}
}

// Get handles GET /api/settings.
func (h *SettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	settings, err := h.settingsSvc.GetSettings(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed querying settings: "+err.Error())
		return
	}
	// Redact sensitive service account passwords when returning to UI
	sanitized := *settings
	if sanitized.Auth.LDAP.BindPassword != "" {
		sanitized.Auth.LDAP.BindPassword = "••••••••••••"
	}
	if sanitized.Auth.ActiveDirectory.BindPassword != "" {
		sanitized.Auth.ActiveDirectory.BindPassword = "••••••••••••"
	}
	writeJSON(w, http.StatusOK, sanitized)
}

// Update handles PUT /api/settings.
func (h *SettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	var newSettings domain.SystemSettings
	if err := json.NewDecoder(r.Body).Decode(&newSettings); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid settings payload: "+err.Error())
		return
	}

	// If masked passwords were sent, preserve current active passwords
	current, _ := h.settingsSvc.GetSettings(r.Context())
	if current != nil {
		if newSettings.Auth.LDAP.BindPassword == "••••••••••••" {
			newSettings.Auth.LDAP.BindPassword = current.Auth.LDAP.BindPassword
		}
		if newSettings.Auth.ActiveDirectory.BindPassword == "••••••••••••" {
			newSettings.Auth.ActiveDirectory.BindPassword = current.Auth.ActiveDirectory.BindPassword
		}
	}

	if err := h.settingsSvc.UpdateSettings(r.Context(), newSettings); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed updating settings: "+err.Error())
		return
	}

	if h.dnsmasq != nil {
		_ = h.dnsmasq.UpdateNetworkSettings(r.Context(), newSettings.Network, newSettings.General.ProvisioningInterface)
	}

	writeJSON(w, http.StatusOK, map[string]bool{"saved": true})
}

// TestDirectory handles POST /api/settings/test-directory.
func (h *SettingsHandler) TestDirectory(w http.ResponseWriter, r *http.Request) {
	var req domain.DirectoryTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid test request: "+err.Error())
		return
	}

	// If password was redacted in UI, substitute currently saved credentials
	current, _ := h.settingsSvc.GetSettings(r.Context())
	if current != nil {
		if req.LDAP != nil && req.LDAP.BindPassword == "••••••••••••" {
			req.LDAP.BindPassword = current.Auth.LDAP.BindPassword
		}
		if req.ActiveDirectory != nil && req.ActiveDirectory.BindPassword == "••••••••••••" {
			req.ActiveDirectory.BindPassword = current.Auth.ActiveDirectory.BindPassword
		}
	}

	result := h.settingsSvc.TestDirectory(r.Context(), req)
	writeJSON(w, http.StatusOK, result)
}
