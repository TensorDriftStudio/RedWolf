package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

func TestSQLiteRepository_Lifecycle(t *testing.T) {
	ctx := context.Background()

	// Use temporary in-memory database for isolation
	repo, err := NewRepository(":memory:")
	if err != nil {
		t.Fatalf("failed to create in-memory repo: %v", err)
	}
	defer repo.Close()

	testNode := &domain.ServerNode{
		ID:           "node-001",
		Vendor:       domain.VendorDell,
		Model:        "PowerEdge R640",
		SerialNumber: "SRV-TEST-9988",
		FirmwareMode: domain.FirmwareUEFI,
		BIOSVersion:  "2.18.1",
		Status:       domain.NodeStatusDiscovering,
		CPU: domain.CPUInfo{
			Model:        "Intel Xeon Gold 6248R",
			Sockets:      2,
			TotalThreads: 48,
		},
		Memory: domain.MemoryInfo{
			TotalBytes: 137438953472,
			TotalHuman: "128 GiB",
		},
		Storage: []domain.StorageDevice{
			{
				Name:      "nvme0n1",
				Path:      "/dev/nvme0n1",
				ByID:      "/dev/disk/by-id/nvme-SAMSUNG_MZQL21T9",
				SizeBytes: 1920383410176,
				SizeHuman: "1.92 TB",
				Type:      "nvme",
			},
		},
		NICs: []domain.NetworkInterface{
			{
				Name:    "eno1",
				MAC:     "ac:1f:6b:11:22:33",
				IsBoot:  true,
				Carrier: true,
			},
			{
				Name:    "eno2",
				MAC:     "ac:1f:6b:11:22:34",
				IsBoot:  false,
				Carrier: false,
			},
		},
		BMC: domain.BMCInfo{
			Vendor:   "Dell Inc.",
			IP:       "192.168.100.45",
			PortMode: domain.PortModeDedicated,
		},
		DiscoveredAt: time.Now().UTC(),
	}

	// 1. Test Save
	if err := repo.Save(ctx, testNode); err != nil {
		t.Fatalf("failed to save node: %v", err)
	}

	// 2. Test GetByID
	fetched, err := repo.GetByID(ctx, "node-001")
	if err != nil {
		t.Fatalf("failed to get node by ID: %v", err)
	}
	if fetched.SerialNumber != testNode.SerialNumber {
		t.Errorf("expected serial %s, got %s", testNode.SerialNumber, fetched.SerialNumber)
	}

	// 3. Test GetByMAC (both boot MAC and secondary NIC)
	byBootMAC, err := repo.GetByMAC(ctx, "ac:1f:6b:11:22:33")
	if err != nil {
		t.Fatalf("failed to get node by boot MAC: %v", err)
	}
	if byBootMAC.ID != "node-001" {
		t.Errorf("expected ID node-001, got %s", byBootMAC.ID)
	}

	bySecMAC, err := repo.GetByMAC(ctx, "ac:1f:6b:11:22:34")
	if err != nil {
		t.Fatalf("failed to get node by secondary MAC: %v", err)
	}
	if bySecMAC.ID != "node-001" {
		t.Errorf("expected ID node-001, got %s", bySecMAC.ID)
	}

	// 4. Test UpdateStatus
	if err := repo.UpdateStatus(ctx, "node-001", domain.NodeStatusReady); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}
	updated, err := repo.GetByID(ctx, "node-001")
	if err != nil {
		t.Fatalf("failed to get updated node: %v", err)
	}
	if updated.Status != domain.NodeStatusReady {
		t.Errorf("expected status READY, got %s", updated.Status)
	}

	// 5. Test UpdateProvisioningState
	provState := &domain.ProvisioningState{
		Progress: 45,
		Stage:    "Streaming OS Image",
		Logs:     []string{"Connected to repository"},
	}
	if err := repo.UpdateProvisioningState(ctx, "node-001", provState); err != nil {
		t.Fatalf("failed to update provisioning state: %v", err)
	}
	withProv, _ := repo.GetByID(ctx, "node-001")
	if withProv.ProvisioningState == nil || withProv.ProvisioningState.Progress != 45 {
		t.Errorf("expected provisioning progress 45, got %+v", withProv.ProvisioningState)
	}

	// 6. Test List
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("failed to list nodes: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 node in list, got %d", len(list))
	}

	// 7. Test Delete
	if err := repo.Delete(ctx, "node-001"); err != nil {
		t.Fatalf("failed to delete node: %v", err)
	}
	_, err = repo.GetByID(ctx, "node-001")
	if err == nil {
		t.Errorf("expected error after delete, got nil")
	}
}
