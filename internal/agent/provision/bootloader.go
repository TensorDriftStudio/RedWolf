package provision

import (
	"bufio"
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
	partNum := detectEFIPartition(ctx, realDisk, osType)

	// Register boot entry on detected EFI system partition
	cmd := exec.CommandContext(ctx, "efibootmgr",
		"-c",
		"-d", realDisk,
		"-p", fmt.Sprintf("%d", partNum),
		"-L", label,
		"-l", loaderPath,
	)

	out, err := cmd.CombinedOutput()
	var bootmgrOut string
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
		fallbackOut, _ := fallbackCmd.CombinedOutput()
		bootmgrOut = string(fallbackOut)
	} else {
		bootmgrOut = string(out)
		slog.InfoContext(ctx, "uefi bootloader registered successfully",
			"target_drive", realDisk,
			"part_num", partNum,
			"loader", loaderPath,
		)
	}

	// Read fresh NVRAM state to prioritize local disk in BootOrder
	queryCmd := exec.CommandContext(ctx, "efibootmgr")
	if freshOut, err := queryCmd.CombinedOutput(); err == nil && len(freshOut) > 0 {
		bootmgrOut = string(freshOut)
	}

	if err := PrioritizeBootOrder(ctx, bootmgrOut, label); err != nil {
		slog.WarnContext(ctx, "could not prioritize uefi boot order in NVRAM (system may require BIOS boot order adjustment)",
			"error", err,
			"label", label,
		)
	}

	return nil
}

// PrioritizeBootOrder inspects efibootmgr output, identifies the BootXXXX number for label,
// and ensures it is placed first in the active BootOrder.
func PrioritizeBootOrder(ctx context.Context, efibootmgrOutput string, label string) error {
	entryID, bootOrder := parseBootmgrState(efibootmgrOutput, label)
	if entryID == "" {
		return fmt.Errorf("boot entry matching label %q not found in efibootmgr output", label)
	}

	newOrder := reorderBootOrder(entryID, bootOrder)
	if newOrder == "" {
		return fmt.Errorf("failed to compute new boot order")
	}

	cmd := exec.CommandContext(ctx, "efibootmgr", "-o", newOrder)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set BootOrder %s: %w (output: %s)", newOrder, err, string(out))
	}

	// Also set BootNext for immediate reboot into the new OS
	nextCmd := exec.CommandContext(ctx, "efibootmgr", "-n", entryID)
	_ = nextCmd.Run()

	slog.InfoContext(ctx, "uefi boot order prioritized successfully in NVRAM",
		"boot_entry", entryID,
		"new_order", newOrder,
	)
	return nil
}

// parseBootmgrState extracts the target entry ID and existing BootOrder from efibootmgr text output.
func parseBootmgrState(output string, label string) (entryID string, bootOrder []string) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "BootOrder:") {
			orderStr := strings.TrimSpace(strings.TrimPrefix(line, "BootOrder:"))
			if orderStr != "" {
				for _, part := range strings.Split(orderStr, ",") {
					trimmed := strings.TrimSpace(part)
					if trimmed != "" {
						bootOrder = append(bootOrder, trimmed)
					}
				}
			}
			continue
		}

		// Look for lines like "Boot0005* RedWolf (almalinux9)" or "Boot0002 RedWolf (debian12)"
		if strings.HasPrefix(line, "Boot") && strings.Contains(line, label) {
			rest := strings.TrimPrefix(line, "Boot")
			// The next 4 chars should be hex digits
			if len(rest) >= 4 {
				cand := rest[:4]
				isHex := true
				for _, r := range cand {
					if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
						isHex = false
						break
					}
				}
				if isHex {
					entryID = cand
				}
			}
		}
	}
	return entryID, bootOrder
}

// reorderBootOrder returns a comma-separated boot order string with entryID at index 0.
func reorderBootOrder(entryID string, currentOrder []string) string {
	if entryID == "" {
		return strings.Join(currentOrder, ",")
	}
	var newOrder []string
	newOrder = append(newOrder, entryID)
	for _, item := range currentOrder {
		if !strings.EqualFold(item, entryID) {
			newOrder = append(newOrder, item)
		}
	}
	return strings.Join(newOrder, ",")
}

func detectEFIPartition(ctx context.Context, realDisk string, osType ...domain.OperatingSystem) int {
	var targetOS domain.OperatingSystem
	if len(osType) > 0 {
		targetOS = osType[0]
	}
	defaultPart := 1
	if strings.Contains(strings.ToLower(string(targetOS)), "debian") {
		defaultPart = 15
	} else if strings.Contains(strings.ToLower(string(targetOS)), "almalinux") {
		defaultPart = 2
	}

	cmd := exec.CommandContext(ctx, "lsblk", "-J", "-b", "-o", "NAME,PATH,PARTTYPE,FSTYPE,LABEL,TYPE", realDisk)
	out, err := cmd.Output()
	if err != nil {
		return defaultPart
	}

	partNum := parseEFIPartitionFromJSON(out)
	if partNum > 0 {
		return partNum
	}
	return defaultPart
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
		return 0
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

	return 0
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
