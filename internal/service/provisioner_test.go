package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestProvisioner_ImageCatalogPreFlight(t *testing.T) {
	ctx := context.Background()

	repo, err := sqlite.NewRepository(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}
	defer repo.Close()

	bus := event.NewBroadcaster()
	prov := NewProvisioner(repo, bus)

	tmpDir := t.TempDir()
	catalog := NewImageCatalogService(tmpDir)
	prov.SetImageCatalog(catalog)

	node := &domain.ServerNode{
		ID:           "node-preflight-test",
		Vendor:       domain.VendorSupermicro,
		Model:        "SYS-1029P-WTR",
		SerialNumber: "SM-1029-99",
		Status:       domain.NodeStatusReady,
		NICs: []domain.NetworkInterface{
			{Name: "eno1", MAC: "00:25:90:ab:cd:ef", IsBoot: true, Carrier: true},
		},
		DiscoveredAt: time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	_, err = prov.RegisterDiscoveredNode(ctx, node)
	if err != nil {
		t.Fatalf("failed to register node: %v", err)
	}

	cfg := domain.DeploymentConfig{
		NodeID:          node.ID,
		OS:              domain.OSAlmaLinux9,
		TargetDrivePath: "/dev/disk/by-id/nvme-TEST",
		RootPassword:    "TestPass1234!",
		NetworkMode:     domain.NetworkModeDHCP,
	}

	// 1. Attempt deployment without cached OS image - must fail preflight
	err = prov.InitiateDeployment(ctx, cfg)
	if err == nil {
		t.Fatalf("expected InitiateDeployment to fail when image is missing from catalog")
	}
	if !strings.Contains(err.Error(), "not cached on appliance") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// Verify node status remained READY
	persisted, err := prov.GetNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("failed fetching node: %v", err)
	}
	if persisted.Status != domain.NodeStatusReady {
		t.Fatalf("expected node status to remain READY, got: %s", persisted.Status)
	}

	// 2. Create the image file in the catalog directory
	dummyImagePath := filepath.Join(tmpDir, "almalinux-9-genericcloud.raw.zstd")
	if err := os.WriteFile(dummyImagePath, []byte("dummy raw zstd stream"), 0644); err != nil {
		t.Fatalf("failed creating dummy image file: %v", err)
	}

	// 3. Re-attempt deployment - should succeed now
	if err := prov.InitiateDeployment(ctx, cfg); err != nil {
		t.Fatalf("InitiateDeployment failed with image present: %v", err)
	}

	// Verify node transitioned to PROVISIONING
	persisted, err = prov.GetNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("failed fetching node: %v", err)
	}
	if persisted.Status != domain.NodeStatusProvisioning {
		t.Fatalf("expected node status to be PROVISIONING, got: %s", persisted.Status)
	}

	task, err := prov.GetPendingTask(ctx, node.ID)
	if err != nil {
		t.Fatalf("failed getting pending task: %v", err)
	}
	if task.ImageURL != "/assets/images/almalinux-9-genericcloud.raw.zstd" {
		t.Fatalf("unexpected ImageURL: %s", task.ImageURL)
	}
}

func TestProvisioner_DeploymentFailureTransition(t *testing.T) {
	ctx := context.Background()

	repo, err := sqlite.NewRepository(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}
	defer repo.Close()

	bus := event.NewBroadcaster()
	prov := NewProvisioner(repo, bus)

	node := &domain.ServerNode{
		ID:           "node-failure-test",
		Vendor:       domain.VendorDell,
		Model:        "PowerEdge R740",
		SerialNumber: "DELL-FAIL-01",
		Status:       domain.NodeStatusReady,
		NICs: []domain.NetworkInterface{
			{Name: "eno1", MAC: "ac:1f:6b:aa:bb:cc", IsBoot: true, Carrier: true},
		},
		DiscoveredAt: time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	_, err = prov.RegisterDiscoveredNode(ctx, node)
	if err != nil {
		t.Fatalf("failed to register node: %v", err)
	}

	cfg := domain.DeploymentConfig{
		NodeID:          node.ID,
		OS:              domain.OSAlmaLinux9,
		TargetDrivePath: "/dev/nvme0n1",
		RootPassword:    "EnterpriseSafe123!",
		NetworkMode:     domain.NetworkModeDHCP,
	}

	if err := prov.InitiateDeployment(ctx, cfg); err != nil {
		t.Fatalf("failed initiating deployment: %v", err)
	}

	// Report failure from agent
	err = prov.UpdateProgress(ctx, node.ID, 0, "Deployment Failed", "Image streaming failed: broken pipe")
	if err != nil {
		t.Fatalf("failed updating progress with error: %v", err)
	}

	failedNode, err := prov.GetNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("failed retrieving node: %v", err)
	}
	if failedNode.Status != domain.NodeStatusError {
		t.Fatalf("expected node status ERROR, got: %s", failedNode.Status)
	}

	// Verify task was cleared
	_, err = prov.GetPendingTask(ctx, node.ID)
	if err == nil {
		t.Fatalf("expected pending task to be cleared on failure")
	}

	// Verify reset recovers node back to READY
	resetNode, err := prov.ResetNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("failed to reset node: %v", err)
	}
	if resetNode.Status != domain.NodeStatusReady {
		t.Fatalf("expected reset node status to be READY, got: %s", resetNode.Status)
	}
}


