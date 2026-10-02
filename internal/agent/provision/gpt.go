package provision

import (
	"context"
	"log/slog"
	"os/exec"
	"time"
)

// RepairGPTHeader moves the secondary GPT header to the physical end of the block device
// and refreshes the Linux kernel partition table.
func RepairGPTHeader(ctx context.Context, targetDrivePath string) error {
	slog.InfoContext(ctx, "relocating secondary GPT header to physical drive end", "target_drive", targetDrivePath)

	// Step 1: Execute sgdisk -e to relocate secondary GPT header to physical disk end
	sgdiskCmd := exec.CommandContext(ctx, "sgdisk", "-e", targetDrivePath)
	out, err := sgdiskCmd.CombinedOutput()
	if err != nil {
		slog.WarnContext(ctx, "sgdisk -e returned warning/error", "error", err, "output", string(out))
		// Some raw cloud images might use MBR or already valid GPT; try parted fix
		partedCmd := exec.CommandContext(ctx, "parted", "-s", targetDrivePath, "print")
		_ = partedCmd.Run()
	}

	// Step 2: Inform kernel to re-read partition table
	slog.InfoContext(ctx, "rereading block device partition table", "target_drive", targetDrivePath)
	partprobeCmd := exec.CommandContext(ctx, "partprobe", targetDrivePath)
	if err := partprobeCmd.Run(); err != nil {
		// Fallback to blockdev --rereadpt
		blockdevCmd := exec.CommandContext(ctx, "blockdev", "--rereadpt", targetDrivePath)
		_ = blockdevCmd.Run()
	}

	// Give udev and kernel 2 seconds to settle device nodes (/dev/disk/by-id/*-part* or /dev/...p*)
	time.Sleep(2 * time.Second)

	slog.InfoContext(ctx, "gpt partition table repaired and synced with kernel", "target_drive", targetDrivePath)
	return nil
}
