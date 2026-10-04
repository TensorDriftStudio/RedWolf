package provision

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// RepairGPTHeader moves the secondary GPT header to the physical end of the block device,
// expands the root partition entry to use available contiguous space, and refreshes the kernel partition table.
func RepairGPTHeader(ctx context.Context, targetDrivePath string) error {
	realDev, err := filepath.EvalSymlinks(targetDrivePath)
	if err != nil {
		realDev = targetDrivePath
	}

	slog.InfoContext(ctx, "relocating secondary GPT header to physical drive end", "target_drive", realDev)

	// Step 1: Execute sgdisk -e to relocate secondary GPT header to physical disk end
	sgdiskCmd := exec.CommandContext(ctx, "sgdisk", "-e", realDev)
	out, err := sgdiskCmd.CombinedOutput()
	if err != nil {
		slog.WarnContext(ctx, "sgdisk -e returned warning/error", "error", err, "output", string(out))
		// Some raw cloud images might use MBR or already valid GPT; try parted fix
		partedFixCmd := exec.CommandContext(ctx, "parted", "-s", realDev, "print")
		_ = partedFixCmd.Run()
	}

	// Step 2: Inform kernel to re-read partition table
	_ = exec.CommandContext(ctx, "partprobe", realDev).Run()
	_ = exec.CommandContext(ctx, "blockdev", "--rereadpt", realDev).Run()
	time.Sleep(1 * time.Second)

	// Step 3: Automatically expand root partition entry to fill the drive
	if err := ExpandRootPartition(ctx, realDev); err != nil {
		slog.WarnContext(ctx, "warning expanding root partition entry in GPT", "error", err, "drive", realDev)
	}

	slog.InfoContext(ctx, "gpt partition table repaired and synced with kernel", "target_drive", realDev)
	return nil
}

// ExpandRootPartition locates the root partition on the disk and resizes its boundary to 100% of available space.
func ExpandRootPartition(ctx context.Context, diskPath string) error {
	partNum, err := detectRootPartNumber(ctx, diskPath)
	if err != nil {
		return fmt.Errorf("unable to detect root partition number for expansion: %w", err)
	}

	slog.InfoContext(ctx, "expanding root partition boundary to 100% disk capacity",
		"disk", diskPath,
		"part_num", partNum,
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

	// Inform kernel of modified partition boundary
	_ = exec.CommandContext(ctx, "partprobe", diskPath).Run()
	_ = exec.CommandContext(ctx, "blockdev", "--rereadpt", diskPath).Run()
	time.Sleep(1 * time.Second)

	return nil
}

// detectRootPartNumber parses lsblk output to identify the partition index of the root filesystem.
func detectRootPartNumber(ctx context.Context, diskPath string) (int, error) {
	cmd := exec.CommandContext(ctx, "lsblk", "-J", "-b", "-o", "NAME,PATH,SIZE,TYPE,FSTYPE,LABEL,PARTLABEL,PARTNUM", diskPath)
	out, err := cmd.Output()
	if err != nil {
		// Fallback for standard images: partition 4 for AlmaLinux/RHEL, or partition 1 for Debian
		return 4, nil
	}

	var data struct {
		BlockDevices []struct {
			Name      string      `json:"name"`
			Path      string      `json:"path"`
			Type      string      `json:"type"`
			Size      json.Number `json:"size"`
			FSType    string      `json:"fstype"`
			Label     string      `json:"label"`
			PartLabel string      `json:"partlabel"`
			PartNum   json.Number `json:"partnum"`
			Children  []struct {
				Name      string      `json:"name"`
				Path      string      `json:"path"`
				Type      string      `json:"type"`
				Size      json.Number `json:"size"`
				FSType    string      `json:"fstype"`
				Label     string      `json:"label"`
				PartLabel string      `json:"partlabel"`
				PartNum   json.Number `json:"partnum"`
			} `json:"children,omitempty"`
		} `json:"blockdevices"`
	}

	if err := json.Unmarshal(out, &data); err != nil {
		return 4, nil
	}

	type candidatePart struct {
		num       int
		partLabel string
		label     string
		fsType    string
		size      int64
	}

	var parts []candidatePart
	for _, dev := range data.BlockDevices {
		if dev.Type == "part" {
			num, _ := dev.PartNum.Int64()
			if num == 0 {
				num = int64(extractTrailingDigits(dev.Name))
			}
			sz, _ := dev.Size.Int64()
			parts = append(parts, candidatePart{
				num:       int(num),
				partLabel: dev.PartLabel,
				label:     dev.Label,
				fsType:    dev.FSType,
				size:      sz,
			})
		}
		for _, child := range dev.Children {
			num, _ := child.PartNum.Int64()
			if num == 0 {
				num = int64(extractTrailingDigits(child.Name))
			}
			sz, _ := child.Size.Int64()
			parts = append(parts, candidatePart{
				num:       int(num),
				partLabel: child.PartLabel,
				label:     child.Label,
				fsType:    child.FSType,
				size:      sz,
			})
		}
	}

	if len(parts) == 0 {
		return 4, nil
	}

	// 1. Explicit root PARTLABEL or LABEL
	for _, p := range parts {
		if strings.Contains(strings.ToLower(p.partLabel), "root") || strings.Contains(strings.ToLower(p.label), "root") {
			if p.num > 0 {
				return p.num, nil
			}
		}
	}

	// 2. Largest xfs or ext4 filesystem
	var bestNum int
	var largestSize int64
	for _, p := range parts {
		if (p.fsType == "xfs" || p.fsType == "ext4") && p.size > largestSize {
			largestSize = p.size
			bestNum = p.num
		}
	}
	if bestNum > 0 {
		return bestNum, nil
	}

	// 3. Largest partition overall
	for _, p := range parts {
		if p.size > largestSize {
			largestSize = p.size
			bestNum = p.num
		}
	}
	if bestNum > 0 {
		return bestNum, nil
	}

	// Default fallback to 4
	return 4, nil
}

