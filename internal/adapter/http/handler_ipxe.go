package http

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/service"
)

// IPXEHandler serves dynamic iPXE scripts conditioned on node state to prevent boot loops.
type IPXEHandler struct {
	prov      *service.Provisioner
	serverURL string
}

// NewIPXEHandler creates an initialized iPXE endpoint handler.
func NewIPXEHandler(prov *service.Provisioner, serverURL string) *IPXEHandler {
	return &IPXEHandler{
		prov:      prov,
		serverURL: serverURL,
	}
}

// ServeHTTP inspects the client MAC query parameter and renders the appropriate iPXE script.
func (h *IPXEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mac := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mac")))
	ctx := r.Context()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	if mac == "" {
		slog.WarnContext(ctx, "iPXE boot request without mac query parameter; defaulting to discovery")
		h.renderDiscoveryScript(w)
		return
	}

	node, err := h.prov.GetNodeByMAC(ctx, mac)
	if err != nil {
		if domain.ErrNodeNotFoundIs(err) {
			slog.InfoContext(ctx, "unknown node booting via iPXE; streaming discovery agent", "mac", mac)
			h.renderDiscoveryScript(w)
			return
		}
		slog.ErrorContext(ctx, "error querying node by mac in ipxe handler", "mac", mac, "error", err)
		h.renderDiscoveryScript(w)
		return
	}

	slog.InfoContext(ctx, "rendering dynamic iPXE script", "node_id", node.ID, "status", node.Status, "mac", mac)

	switch node.Status {
	case domain.NodeStatusActive:
		// Break the PXE boot loop by dropping out of iPXE to the local UEFI/BIOS drive
		h.renderLocalBootScript(w, node)

	case domain.NodeStatusProvisioning:
		// Node is actively deploying; stream provisioning ramdisk
		h.renderProvisioningScript(w, node)

	case domain.NodeStatusDiscovering, domain.NodeStatusReady, domain.NodeStatusError:
		fallthrough
	default:
		// Boot into in-memory discovery agent
		h.renderDiscoveryScript(w)
	}
}

func (h *IPXEHandler) renderLocalBootScript(w http.ResponseWriter, node *domain.ServerNode) {
	script := fmt.Sprintf(`#!ipxe
echo ========================================================
echo RedWolf Provisioning Engine: Node is ACTIVE in Production
echo Serial: %s  Model: %s
echo Exiting iPXE. Booting from local storage...
echo ========================================================
exit 1
`, node.SerialNumber, node.Model)
	_, _ = w.Write([]byte(script))
}

func (h *IPXEHandler) renderDiscoveryScript(w http.ResponseWriter) {
	script := fmt.Sprintf(`#!ipxe
echo ========================================================
echo RedWolf Discovery Agent (In-Memory RAMdisk)
echo Streaming kernel and discovery environment over HTTP...
echo ========================================================
kernel %s/assets/discovery/vmlinuz console=tty0 console=ttyS0,115200n8 initrd=initramfs.img redwolf.server=%s
initrd %s/assets/discovery/initramfs.img
boot
`, h.serverURL, h.serverURL, h.serverURL)
	_, _ = w.Write([]byte(script))
}

func (h *IPXEHandler) renderProvisioningScript(w http.ResponseWriter, node *domain.ServerNode) {
	script := fmt.Sprintf(`#!ipxe
echo ========================================================
echo RedWolf Bare-Metal Provisioning Engine
echo Node ID: %s
echo Target Drive: %s
echo ========================================================
kernel %s/assets/discovery/vmlinuz console=tty0 console=ttyS0,115200n8 initrd=initramfs.img redwolf.mode=provision redwolf.node_id=%s redwolf.server=%s
initrd %s/assets/discovery/initramfs.img
boot
`, node.ID, node.ProvisioningState.TargetDrive, h.serverURL, node.ID, h.serverURL, h.serverURL)
	_, _ = w.Write([]byte(script))
}
