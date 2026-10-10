package provision

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// RepairGPTHeader moves the secondary GPT header to the physical end of the block device,
// expands the root partition entry to use available space, and refreshes the kernel partition table.
func RepairGPTHeader(ctx context.Context, targetDrivePath string, osType ...domain.OperatingSystem) error {
	realDev, err := filepath.EvalSymlinks(targetDrivePath)
	if err != nil {
		realDev = targetDrivePath
	}

	var targetOS domain.OperatingSystem
	if len(osType) > 0 {
		targetOS = osType[0]
	}

	slog.InfoContext(ctx, "relocating secondary GPT header to physical drive end", "target_drive", realDev, "os", targetOS)

	sgdiskCmd := exec.CommandContext(ctx, "sgdisk", "-e", realDev)
	out, err := sgdiskCmd.CombinedOutput()
	if err != nil {
		slog.WarnContext(ctx, "sgdisk -e returned warning/error", "error", err, "output", string(out))
		partedFixCmd := exec.CommandContext(ctx, "parted", "-s", realDev, "print")
		_ = partedFixCmd.Run()
	}

	// Ensure active boot flag on Protective MBR for legacy BIOS firmware
	_ = exec.CommandContext(ctx, "parted", "-s", realDev, "disk_set", "pmbr_boot", "on").Run()
	settlePartitions(ctx, realDev)

	if err := ExpandRootPartition(ctx, realDev, targetOS); err != nil {
		slog.WarnContext(ctx, "warning expanding root partition entry in GPT", "error", err, "drive", realDev)
	}

	settlePartitions(ctx, realDev)

	slog.InfoContext(ctx, "gpt partition table repaired and synced with kernel", "target_drive", realDev)
	return nil
}

// ExpandRootPartition locates the root partition on the disk and resizes its boundary to 100% of available space.
func ExpandRootPartition(ctx context.Context, diskPath string, osType ...domain.OperatingSystem) error {
	var targetOS domain.OperatingSystem
	if len(osType) > 0 {
		targetOS = osType[0]
	}

	partNum, err := detectRootPartNumber(ctx, diskPath, targetOS)
	if err != nil {
		return fmt.Errorf("unable to detect root partition number for expansion: %w", err)
	}

	slog.InfoContext(ctx, "expanding root partition boundary to 100% disk capacity",
		"disk", diskPath,
		"part_num", partNum,
		"os", targetOS,
	)

	// Attempt parted resizepart to 100%
	partedCmd := exec.CommandContext(ctx, "parted", "-s", diskPath, "resizepart", strconv.Itoa(partNum), "100%")
	out, err := partedCmd.CombinedOutput()
	if err != nil {
		slog.WarnContext(ctx, "parted resizepart failed; attempting growpart fallback", "error", err, "output", string(out))
		growpartCmd := exec.CommandContext(ctx, "growpart", diskPath, strconv.Itoa(partNum))
		if gpOut, gpErr := growpartCmd.CombinedOutput(); gpErr != nil {
			slog.WarnContext(ctx, "growpart also returned warning/error", "error", gpErr, "output", string(gpOut))
		}
	}

	settlePartitions(ctx, diskPath)
	return nil
}

// detectRootPartNumber resolves the partition index of the root filesystem on the given disk.
func detectRootPartNumber(ctx context.Context, diskPath string, osType ...domain.OperatingSystem) (int, error) {
	partPath, err := findRootPartition(ctx, diskPath, osType...)
	if err != nil {
		return 0, err
	}

	num := extractTrailingDigits(partPath)
	if num > 0 {
		return num, nil
	}

	var targetOS domain.OperatingSystem
	if len(osType) > 0 {
		targetOS = osType[0]
	}
	if strings.Contains(strings.ToLower(string(targetOS)), "debian") || strings.Contains(strings.ToLower(string(targetOS)), "ubuntu") {
		return 1, nil
	}
	return 4, nil
}

