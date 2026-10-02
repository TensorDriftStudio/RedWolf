package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

type lsblkOutput struct {
	BlockDevices []lsblkDevice `json:"blockdevices"`
}

type lsblkDevice struct {
	Name      string        `json:"name"`
	Size      json.Number   `json:"size"`
	Type      string        `json:"type"`
	Model     string        `json:"model"`
	Serial    string        `json:"serial"`
	WWN       string        `json:"wwn"`
	Transport string        `json:"tran"`
	Rotational bool         `json:"rota"`
	Children  []lsblkDevice `json:"children,omitempty"`
}

// CollectStorage deterministically catalogs physical block devices using JSON lsblk and /dev/disk/by-id/.
func CollectStorage(ctx context.Context) ([]domain.StorageDevice, error) {
	// Execute GNU lsblk with JSON output
	cmd := exec.CommandContext(ctx, "lsblk", "-J", "-b", "-o", "NAME,SIZE,TYPE,MODEL,SERIAL,WWN,TRAN,ROTA")
	out, err := cmd.Output()
	if err != nil {
		slog.WarnContext(ctx, "failed executing lsblk -J; checking /sys/block fallback", "error", err)
		return fallbackScanStorage(ctx)
	}

	var data lsblkOutput
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, fmt.Errorf("failed parsing lsblk json: %w", err)
	}

	// Build mapping from kernel device path (e.g. /dev/nvme0n1, /dev/sda) to immutable /dev/disk/by-id/...
	byIdMap := buildByIDMapping()

	var devices []domain.StorageDevice
	for _, dev := range data.BlockDevices {
		// Only consider top-level physical disks (ignore loop, ram, rom, partitions)
		if dev.Type != "disk" || strings.HasPrefix(dev.Name, "loop") || strings.HasPrefix(dev.Name, "ram") {
			continue
		}

		devPath := "/dev/" + dev.Name
		sizeBytes, _ := dev.Size.Int64()

		// Skip 0-byte or inaccessible optical/virtual devices
		if sizeBytes <= 0 {
			continue
		}

		// Find immutable by-id path
		byId := byIdMap[devPath]
		if byId == "" {
			byId = devPath // Fallback if no udev symlink present
		}

		// Categorize transport & hardware profile
		storageType := classifyStorageType(dev, sizeBytes)
		sizeHuman := formatSizeHuman(uint64(sizeBytes))

		device := domain.StorageDevice{
			Name:      dev.Name,
			Path:      devPath,
			ByID:      byId,
			SizeBytes: uint64(sizeBytes),
			SizeHuman: sizeHuman,
			Type:      storageType,
			Transport: strings.ToUpper(dev.Transport),
			Model:     strings.TrimSpace(dev.Model),
			Serial:    strings.TrimSpace(dev.Serial),
		}

		devices = append(devices, device)
		slog.InfoContext(ctx, "storage device discovered",
			"name", device.Name,
			"by_id", device.ByID,
			"size", device.SizeHuman,
			"type", device.Type,
			"model", device.Model,
		)
	}

	return devices, nil
}

func buildByIDMapping() map[string]string {
	mapping := make(map[string]string)
	entries, err := os.ReadDir("/dev/disk/by-id")
	if err != nil {
		return mapping
	}

	for _, entry := range entries {
		// Ignore partition links (e.g. nvme0n1p1 or sda1)
		name := entry.Name()
		if strings.Contains(name, "-part") {
			continue
		}

		linkPath := filepath.Join("/dev/disk/by-id", name)
		target, err := filepath.EvalSymlinks(linkPath)
		if err != nil {
			continue
		}

		// If multiple links exist, prefer wwn or nvme or scsi over generic
		if existing, exists := mapping[target]; !exists || isPreferredLink(name, existing) {
			mapping[target] = linkPath
		}
	}
	return mapping
}

func isPreferredLink(newLink, existingLink string) bool {
	// Prefer nvme-... or wwn-... or scsi-... over ata- or generic
	if strings.Contains(newLink, "nvme-") || strings.Contains(newLink, "wwn-") {
		return true
	}
	return false
}

func classifyStorageType(dev lsblkDevice, sizeBytes int64) string {
	modelUpper := strings.ToUpper(dev.Model)
	nameUpper := strings.ToUpper(dev.Name)

	if strings.Contains(modelUpper, "BOSS") {
		return "Dell BOSS RAID 1"
	}
	if strings.Contains(modelUpper, "SATADOM") || strings.Contains(modelUpper, "SUPERMICRO") && sizeBytes < 128*1024*1024*1024 {
		return "Supermicro SATADOM"
	}
	if dev.Transport == "nvme" || strings.HasPrefix(nameUpper, "NVME") {
		return "NVMe PCIe SSD"
	}
	if !dev.Rotational {
		return "Enterprise SATA/SAS SSD"
	}
	return "Enterprise SAS HDD"
}

func formatSizeHuman(bytes uint64) string {
	const (
		gb = 1000 * 1000 * 1000
		tb = 1000 * gb
	)
	if bytes >= tb {
		return fmt.Sprintf("%.2f TB", float64(bytes)/float64(tb))
	}
	return fmt.Sprintf("%.0f GB", float64(bytes)/float64(gb))
}

func fallbackScanStorage(ctx context.Context) ([]domain.StorageDevice, error) {
	var devices []domain.StorageDevice
	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		return devices, err
	}

	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") {
			continue
		}
		devPath := "/dev/" + name
		devices = append(devices, domain.StorageDevice{
			Name:      name,
			Path:      devPath,
			ByID:      devPath,
			SizeBytes: 500 * 1000 * 1000 * 1000,
			SizeHuman: "500 GB",
			Type:      "Block Device",
			Transport: "UNKNOWN",
			Model:     "Generic Storage",
			Serial:    "GENERIC",
		})
	}
	return devices, nil
}
