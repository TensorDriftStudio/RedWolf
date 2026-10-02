package service

import (
	"context"
	"testing"
	"time"

	"github.com/tensordriftstudio/redwolf/internal/adapter/event"
	"github.com/tensordriftstudio/redwolf/internal/adapter/sqlite"
	"github.com/tensordriftstudio/redwolf/internal/domain"
)

func TestProvisioner_DeploymentTaskLifecycle(t *testing.T) {
	ctx := context.Background()

	// Initialize in-memory SQLite repo and event bus
	repo, err := sqlite.NewRepository(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}
	defer repo.Close()

	bus := event.NewBroadcaster()
	prov := NewProvisioner(repo, bus)

	// 1. Register a test node
	node := &domain.ServerNode{
		ID:           "test-node-123",
		Vendor:       domain.VendorDell,
		Model:        "PowerEdge R640",
		SerialNumber: "DELL-SRV-01",
		Status:       domain.NodeStatusReady,
		NICs: []domain.NetworkInterface{
			{
				Name:    "eno1",
				MAC:     "ac:1f:6b:12:34:56",
				IsBoot:  true,
				Carrier: true,
			},
		},
		DiscoveredAt: time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	_, err = prov.RegisterDiscoveredNode(ctx, node)
	if err != nil {
		t.Fatalf("failed to register node: %v", err)
	}

	// 2. Initiate deployment
	cfg := domain.DeploymentConfig{
		NodeID:          node.ID,
		OS:              domain.OSAlmaLinux9,
		TargetDrivePath: "/dev/disk/by-id/nvme-SAMSUNG_MZQL2960HCJR-00A07_TEST",
		RootPassword:    "EnterpriseSafe123!",
		NetworkMode:     domain.NetworkModeDHCP,
	}

	err = prov.InitiateDeployment(ctx, cfg)
	if err != nil {
		t.Fatalf("failed initiating deployment: %v", err)
	}

	// 3. Verify node transitioned to PROVISIONING
	updated, err := prov.GetNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("failed retrieving node: %v", err)
	}
	if updated.Status != domain.NodeStatusProvisioning {
		t.Fatalf("expected status PROVISIONING, got: %s", updated.Status)
	}

	// 4. Query pending task by Node ID
	taskByID, err := prov.GetPendingTask(ctx, node.ID)
	if err != nil {
		t.Fatalf("failed getting pending task by ID: %v", err)
	}
	if taskByID.OS != domain.OSAlmaLinux9 {
		t.Fatalf("expected OS AlmaLinux 9, got: %s", taskByID.OS)
	}

	// 5. Query pending task by MAC address
	taskByMAC, err := prov.GetPendingTaskByMAC(ctx, "ac:1f:6b:12:34:56")
	if err != nil {
		t.Fatalf("failed getting pending task by MAC: %v", err)
	}
	if taskByMAC.TaskID != taskByID.TaskID {
		t.Fatalf("task ID mismatch: %s != %s", taskByMAC.TaskID, taskByID.TaskID)
	}

	// 6. Report progress to 100% (Active)
	err = prov.UpdateProgress(ctx, node.ID, 100, "Active in Production", "Reboot complete")
	if err != nil {
		t.Fatalf("failed updating progress: %v", err)
	}

	activeNode, err := prov.GetNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("failed retrieving active node: %v", err)
	}
	if activeNode.Status != domain.NodeStatusActive {
		t.Fatalf("expected status ACTIVE, got: %s", activeNode.Status)
	}

	// 7. Verify pending task is cleared upon completion
	_, err = prov.GetPendingTask(ctx, node.ID)
	if err == nil {
		t.Fatalf("expected pending task to be cleared after 100%% completion")
	}
}
