package http

import (
	"encoding/json"
	"net"
	"net/http"

	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/port"
	"github.com/tensordriftstudio/redwolf/internal/service"
)

// HostInterfaceInfo represents a detected physical or virtual network interface on the appliance.
type HostInterfaceInfo struct {
	Name  string   `json:"name"`
	MAC   string   `json:"mac"`
	IPs   []string `json:"ips"`
	IsUp  bool     `json:"isUp"`
	Flags string   `json:"flags"`
}

// SettingsHandler manages HTTP endpoints for system settings and directory diagnostics.
type SettingsHandler struct {
	settingsSvc  *service.SettingsService
	dnsmasq      port.DNSMasqManager
	imageCatalog *service.ImageCatalogService
}

// NewSettingsHandler creates an initialized SettingsHandler.
func NewSettingsHandler(settingsSvc *service.SettingsService, dnsmasq port.DNSMasqManager, imageCatalog *service.ImageCatalogService) *SettingsHandler {
	return &SettingsHandler{
		settingsSvc:  settingsSvc,
		dnsmasq:      dnsmasq,
		imageCatalog: imageCatalog,
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

	if h.imageCatalog != nil && newSettings.Storage.ImageStorageDir != "" {
		h.imageCatalog.SetImageDir(newSettings.Storage.ImageStorageDir)
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

// GetHostInterfaces handles GET /api/settings/interfaces to assist operator interface selection.
func (h *SettingsHandler) GetHostInterfaces(w http.ResponseWriter, r *http.Request) {
	ifaces, err := net.Interfaces()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed querying host network interfaces: "+err.Error())
		return
	}

	var result []HostInterfaceInfo
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		var ips []string
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok {
				if ipNet.IP.To4() != nil {
					ips = append(ips, ipNet.String())
				}
			}
		}
		result = append(result, HostInterfaceInfo{
			Name:  iface.Name,
			MAC:   iface.HardwareAddr.String(),
			IPs:   ips,
			IsUp:  iface.Flags&net.FlagUp != 0,
			Flags: iface.Flags.String(),
		})
	}
	writeJSON(w, http.StatusOK, result)
}
