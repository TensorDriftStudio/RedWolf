package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	adapterEvent "github.com/tensordriftstudio/redwolf/internal/adapter/event"
	adapterSQLite "github.com/tensordriftstudio/redwolf/internal/adapter/sqlite"
	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/service"
)

func TestIPXEHandler_DynamicBootLoopBreaker(t *testing.T) {
	ctx := context.Background()

	// In-memory sqlite repo for testing
	repo, err := adapterSQLite.NewRepository(":memory:")
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}
	defer repo.Close()

	events := adapterEvent.NewBroadcaster()
	prov := service.NewProvisioner(repo, events)
	handler := NewIPXEHandler(prov, nil, "http://10.10.100.1:8080")

	// 1. Test unknown MAC -> returns discovery script
	req := httptest.NewRequest(http.MethodGet, "/boot.ipxe?mac=11:22:33:44:55:66", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "redwolf.server=http://10.10.100.1:8080") {
		t.Errorf("expected discovery script for unknown MAC, got:\n%s", rec.Body.String())
	}

	// 2. Register node in ACTIVE status
	activeNode := &domain.ServerNode{
		ID:           "node-active-1",
		Vendor:       domain.VendorDell,
		Model:        "PowerEdge R740",
		SerialNumber: "SRV-ACTIVE-01",
		FirmwareMode: domain.FirmwareUEFI,
		Status:       domain.NodeStatusActive,
		NICs: []domain.NetworkInterface{
			{Name: "eno1", MAC: "aa:bb:cc:dd:ee:01", IsBoot: true},
		},
		DiscoveredAt: time.Now().UTC(),
	}
	if err := repo.Save(ctx, activeNode); err != nil {
		t.Fatalf("failed to save active node: %v", err)
	}

	// 3. Test active MAC -> returns exit 1 to drop out to local drive
	reqActive := httptest.NewRequest(http.MethodGet, "/boot.ipxe?mac=aa:bb:cc:dd:ee:01", nil)
	recActive := httptest.NewRecorder()
	handler.ServeHTTP(recActive, reqActive)

	bodyActive := recActive.Body.String()
	if !strings.Contains(bodyActive, "exit 1") {
		t.Errorf("expected exit 1 for ACTIVE node to prevent boot loop, got:\n%s", bodyActive)
	}
	if !strings.Contains(bodyActive, "Exiting iPXE. Booting from local storage...") {
		t.Errorf("expected local storage prompt, got:\n%s", bodyActive)
	}

	// 4. Test provisioning state
	activeNode.Status = domain.NodeStatusProvisioning
	activeNode.ProvisioningState = &domain.ProvisioningState{
		TargetDrive: "/dev/nvme0n1",
	}
	_ = repo.Save(ctx, activeNode)

	reqProv := httptest.NewRequest(http.MethodGet, "/boot.ipxe?mac=aa:bb:cc:dd:ee:01", nil)
	recProv := httptest.NewRecorder()
	handler.ServeHTTP(recProv, reqProv)

	bodyProv := recProv.Body.String()
	if !strings.Contains(bodyProv, "redwolf.mode=provision") {
		t.Errorf("expected provisioning mode flag in iPXE script, got:\n%s", bodyProv)
	}
}
