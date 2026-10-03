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

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// ConfigureBootloader registers the UEFI bootloader in system NVRAM via efibootmgr.
func ConfigureBootloader(ctx context.Context, targetDrivePath string, osType domain.OperatingSystem) error {
	// Resolve symlinks to target real disk block device (e.g. /dev/nvme0n1 or /dev/sda)
	realDisk, err := filepath.EvalSymlinks(targetDrivePath)
	if err != nil {
		realDisk = targetDrivePath
	}

	slog.InfoContext(ctx, "configuring UEFI bootloader in NVRAM",
		"target_drive", targetDrivePath,
		"real_disk", realDisk,
		"os", osType,
	)

	// Determine EFI loader path based on distribution
	loaderPath := `\EFI\BOOT\BOOTX64.EFI`
	switch osType {
	case domain.OSAlmaLinux8, domain.OSAlmaLinux9, domain.OSAlmaLinux10:
		loaderPath = `\EFI\almalinux\shimx64.efi`
	case domain.OSDebian12, domain.OSDebian13:
		loaderPath = `\EFI\debian\shimx64.efi`
	}

	label := fmt.Sprintf("RedWolf (%s)", osType)
	partNum := detectEFIPartition(ctx, realDisk)

	// Register boot entry on detected EFI system partition
	cmd := exec.CommandContext(ctx, "efibootmgr",
		"-c",
		"-d", realDisk,
		"-p", fmt.Sprintf("%d", partNum),
		"-L", label,
		"-l", loaderPath,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.WarnContext(ctx, "efibootmgr returned warning/error (system might be in BIOS mode or virtualized)",
			"error", err,
			"output", string(out),
			"part_num", partNum,
		)
		// Try fallback generic loader path
		fallbackCmd := exec.CommandContext(ctx, "efibootmgr",
			"-c",
			"-d", realDisk,
			"-p", fmt.Sprintf("%d", partNum),
			"-L", label,
			"-l", `\EFI\BOOT\BOOTX64.EFI`,
		)
		_ = fallbackCmd.Run()
	} else {
		slog.InfoContext(ctx, "uefi bootloader registered successfully",
			"target_drive", realDisk,
			"part_num", partNum,
			"loader", loaderPath,
		)
	}

	return nil
}

func detectEFIPartition(ctx context.Context, realDisk string) int {
	cmd := exec.CommandContext(ctx, "lsblk", "-J", "-b", "-o", "NAME,PATH,PARTTYPE,FSTYPE,LABEL,TYPE", realDisk)
	out, err := cmd.Output()
	if err != nil {
		return 1
	}

	return parseEFIPartitionFromJSON(out)
}

func parseEFIPartitionFromJSON(out []byte) int {
	type efiPart struct {
		Name     string    `json:"name"`
		Path     string    `json:"path"`
		Type     string    `json:"type"`
		PartType string    `json:"parttype"`
		FSType   string    `json:"fstype"`
		Label    string    `json:"label"`
		Children []efiPart `json:"children,omitempty"`
	}
	var data struct {
		BlockDevices []efiPart `json:"blockdevices"`
	}
	if err := json.Unmarshal(out, &data); err != nil {
		return 1
	}

	var parts []efiPart
	var collect func(devs []efiPart)
	collect = func(devs []efiPart) {
		for _, d := range devs {
			if d.Type == "part" {
				parts = append(parts, d)
			}
			if len(d.Children) > 0 {
				collect(d.Children)
			}
		}
	}
	collect(data.BlockDevices)

	for _, p := range parts {
		isEFI := strings.EqualFold(p.FSType, "vfat") ||
			strings.Contains(strings.ToLower(p.Label), "efi") ||
			strings.Contains(strings.ToLower(p.Label), "esp") ||
			strings.EqualFold(p.PartType, "c12a7328-f81f-11d2-ba4b-00a0c93ec93b")
		if isEFI {
			num := extractTrailingDigits(p.Name)
			if num > 0 {
				return num
			}
		}
	}

	return 1
}

func extractTrailingDigits(s string) int {
	var digits string
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] >= '0' && s[i] <= '9' {
			digits = string(s[i]) + digits
		} else {
			break
		}
	}
	if digits == "" {
		return 0
	}
	val, err := strconv.Atoi(digits)
	if err != nil {
		return 0
	}
	return val
}
