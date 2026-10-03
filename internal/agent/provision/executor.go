package provision

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"time"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// ProgressReporter sends live updates to the RedWolf Core API.
type ProgressReporter interface {
	Report(ctx context.Context, nodeID string, progress int, stage, logMsg string) error
}

// ExecuteDeployment coordinates the entire bare-metal provisioning lifecycle.
func ExecuteDeployment(ctx context.Context, task *domain.DeploymentTask, bootMAC string, reporter ProgressReporter) error {
	nodeID := task.NodeID
	targetDrive := task.TargetDrivePath

	slog.InfoContext(ctx, "starting execution of bare-metal deployment task",
		"task_id", task.TaskID,
		"node_id", nodeID,
		"os", task.OS,
		"drive", targetDrive,
	)

	// Step 1: Storage Architecture & Topology Preparation (Single disk, mdraid, or LVM)
	_ = reporter.Report(ctx, nodeID, 5, "Preparing block storage target...", "Configuring storage topology and sanitizing block targets...")
	layout, err := SetupStorageArchitecture(ctx, task.Config)
	if err != nil {
		slog.WarnContext(ctx, "storage architecture setup warning; proceeding with fallback", "error", err)
		layout = &StorageLayoutResult{
			TargetDrive: targetDrive,
			ESPDrives:   []string{targetDrive},
		}
	}

	streamTarget := layout.TargetDrive
	if streamTarget == "" {
		streamTarget = targetDrive
	}

	// Step 2: Stream OS image with sparse block writes
	onProgress := func(pct int, writtenBytes int64, msg string) {
		_ = reporter.Report(ctx, nodeID, pct, fmt.Sprintf("Streaming %s...", task.OS), msg)
	}

	if err := StreamImage(ctx, task.ImageURL, streamTarget, onProgress); err != nil {
		errMsg := fmt.Sprintf("Image streaming failed: %v", err)
		_ = reporter.Report(ctx, nodeID, 0, "Deployment Failed", errMsg)
		return fmt.Errorf("streaming error: %w", err)
	}

	// Step 3: Repair secondary GPT header boundary
	_ = reporter.Report(ctx, nodeID, 75, "Expanding secondary GPT header...", "Relocating secondary GPT table to physical drive end")
	if err := RepairGPTHeader(ctx, streamTarget); err != nil {
		slog.WarnContext(ctx, "non-fatal warning during gpt repair", "error", err)
	}

	// Step 4: Direct Cloud-Init NoCloud injection into mounted rootfs
	_ = reporter.Report(ctx, nodeID, 85, "Injecting Cloud-Init NoCloud seed...", "Writing user-data, meta-data, and network-config")
	if err := InjectCloudInit(ctx, streamTarget, task.Config, bootMAC); err != nil {
		errMsg := fmt.Sprintf("Cloud-Init injection failed: %v", err)
		_ = reporter.Report(ctx, nodeID, 0, "Deployment Failed", errMsg)
		return fmt.Errorf("cloud-init injection error: %w", err)
	}

	// Step 5: Register UEFI bootloader in NVRAM for all ESP targets
	_ = reporter.Report(ctx, nodeID, 95, "Registering UEFI boot entry...", "Configuring NVRAM with efibootmgr")
	for _, espTarget := range layout.ESPDrives {
		if err := ConfigureBootloader(ctx, espTarget, task.OS); err != nil {
			slog.WarnContext(ctx, "non-fatal warning during bootloader config", "drive", espTarget, "error", err)
		}
	}

	// Step 6: Finalize and trigger reboot into production OS
	_ = reporter.Report(ctx, nodeID, 100, "Active in Production", "Provisioning complete. Rebooting into installed operating system.")
	slog.InfoContext(ctx, "bare-metal provisioning complete; issuing reboot signal", "node_id", nodeID)

	time.Sleep(2 * time.Second)
	rebootCmd := exec.CommandContext(ctx, "reboot", "-f")
	_ = rebootCmd.Run()

	return nil
}
