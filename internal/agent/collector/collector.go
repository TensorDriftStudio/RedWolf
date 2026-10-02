package collector

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// CollectAll coordinates all hardware discovery routines into a unified domain ServerNode.
func CollectAll(ctx context.Context) (*domain.ServerNode, error) {
	slog.InfoContext(ctx, "starting comprehensive hardware telemetry collection")

	// 1. DMI & Firmware Mode
	dmi, err := CollectDMI(ctx)
	if err != nil {
		slog.WarnContext(ctx, "error during DMI collection", "error", err)
	}

	// 2. Out-of-Band BMC (with Vendor Profile & STP Backoff)
	bmc, err := CollectBMC(ctx, dmi.Vendor)
	if err != nil {
		slog.WarnContext(ctx, "error during BMC collection", "error", err)
	}

	// 3. CPU Topology
	cpu, err := CollectCPU(ctx)
	if err != nil {
		slog.WarnContext(ctx, "error during CPU collection", "error", err)
	}

	// 4. Memory Configuration
	mem, err := CollectMemory(ctx)
	if err != nil {
		slog.WarnContext(ctx, "error during Memory collection", "error", err)
	}

	// 5. Deterministic Storage Devices
	storage, err := CollectStorage(ctx)
	if err != nil {
		slog.WarnContext(ctx, "error during Storage collection", "error", err)
	}

	// 6. Network Interfaces
	nics, err := CollectNetwork(ctx)
	if err != nil {
		slog.WarnContext(ctx, "error during Network collection", "error", err)
	}

	now := time.Now().UTC()
	node := &domain.ServerNode{
		Vendor:       dmi.Vendor,
		Model:        dmi.Model,
		SerialNumber: dmi.SerialNumber,
		FirmwareMode: dmi.FirmwareMode,
		BIOSVersion:  dmi.BIOSVersion,
		Status:       domain.NodeStatusReady,
		CPU:          *cpu,
		Memory:       *mem,
		Storage:      storage,
		NICs:         nics,
		BMC:          *bmc,
		DiscoveredAt: now,
		UpdatedAt:    now,
	}

	if node.BootMAC() == "" {
		return nil, fmt.Errorf("no valid network interfaces with MAC addresses found on host")
	}

	slog.InfoContext(ctx, "hardware telemetry collection complete",
		"vendor", node.Vendor,
		"model", node.Model,
		"serial", node.SerialNumber,
		"boot_mac", node.BootMAC(),
		"storage_count", len(node.Storage),
		"nic_count", len(node.NICs),
	)

	return node, nil
}
