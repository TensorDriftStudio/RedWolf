package provision

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// StorageLayoutResult represents the prepared storage topology for OS deployment.
type StorageLayoutResult struct {
	TargetDrive    string   // Primary block device to stream raw image into (e.g. /dev/md0 or /dev/nvme0n1)
	ESPDrives      []string // Partitions or disks where the UEFI bootloader must be registered
	IsSoftwareRAID bool
	IsLVM          bool
	RAIDDevice     string
	LVMVolumeGroup string
	RootPartition  string
}

// SetupStorageArchitecture provisions disks according to standard, software RAID, or LVM configurations.
func SetupStorageArchitecture(ctx context.Context, cfg domain.DeploymentConfig) (*StorageLayoutResult, error) {
	drives := cfg.Storage.TargetDrives
	if len(drives) == 0 && cfg.TargetDrivePath != "" {
		drives = []string{cfg.TargetDrivePath}
	}

	isRAID1 := cfg.Storage.RAIDLevel == domain.RAIDLevel1 || cfg.PartitioningPreset == domain.PartitioningRAID1
	isRAID0 := cfg.Storage.RAIDLevel == domain.RAIDLevel0
	isRAID10 := cfg.Storage.RAIDLevel == domain.RAIDLevel10

	// 1. Software RAID Execution (RAID 1, RAID 0, or RAID 10)
	if (isRAID1 && len(drives) >= 2) || (isRAID0 && len(drives) >= 2) || (isRAID10 && len(drives) >= 4) {
		slog.InfoContext(ctx, "configuring multi-disk Linux Software RAID array",
			"raid_level", cfg.Storage.RAIDLevel,
			"target_drives", drives,
		)

		raidLevelStr := "1"
		if isRAID0 {
			raidLevelStr = "0"
		} else if isRAID10 {
			raidLevelStr = "10"
		}

		// Stop any existing software RAID arrays that may overlap
		stopCmd := exec.CommandContext(ctx, "mdadm", "--stop", "/dev/md0")
		_ = stopCmd.Run()
		stopScanCmd := exec.CommandContext(ctx, "mdadm", "--stop", "--scan")
		_ = stopScanCmd.Run()

		var espPartitions []string
		var raidMemberPartitions []string

		for _, disk := range drives {
			realDisk, err := filepath.EvalSymlinks(disk)
			if err != nil {
				realDisk = disk
			}

			// Pre-wipe disk signatures and zero disk boundaries
			if err := WipeTargetDisk(ctx, realDisk); err != nil {
				slog.WarnContext(ctx, "pre-wipe warning on raid drive", "drive", realDisk, "error", err)
			}

			// Partition with GPT: Partition 1: ESP (1024 MiB), Partition 2: Linux RAID (Remainder)
			zapCmd := exec.CommandContext(ctx, "sgdisk", "-Z", realDisk)
			_ = zapCmd.Run()

			partCmd := exec.CommandContext(ctx, "sgdisk",
				"-n", "1:2048:+1024M", "-t", "1:ef00", "-c", "1:EFI System Partition",
				"-n", "2:0:0", "-t", "2:fd00", "-c", "2:Linux Software RAID",
				realDisk,
			)
			if out, err := partCmd.CombinedOutput(); err != nil {
				slog.WarnContext(ctx, "sgdisk partition warning on raid drive", "drive", realDisk, "error", err, "output", string(out))
			}

			// Inform kernel of partition layout
			_ = exec.CommandContext(ctx, "partprobe", realDisk).Run()
			time.Sleep(1 * time.Second)

			espPart := resolvePartitionPath(realDisk, 1)
			raidPart := resolvePartitionPath(realDisk, 2)

			espPartitions = append(espPartitions, espPart)
			raidMemberPartitions = append(raidMemberPartitions, raidPart)
		}

		mdDevice := "/dev/md0"
		// Create mdadm RAID array
		mdadmArgs := []string{
			"--create", mdDevice,
			fmt.Sprintf("--level=%s", raidLevelStr),
			fmt.Sprintf("--raid-devices=%d", len(raidMemberPartitions)),
			"--metadata=1.2",
			"--run",
		}
		mdadmArgs = append(mdadmArgs, raidMemberPartitions...)

		createCmd := exec.CommandContext(ctx, "mdadm", mdadmArgs...)
		if out, err := createCmd.CombinedOutput(); err != nil {
			slog.WarnContext(ctx, "mdadm --create failed or returned error", "error", err, "output", string(out))
			// Fallback: if mdadm is not present or failed, fallback to primary drive
			return &StorageLayoutResult{
				TargetDrive:    drives[0],
				ESPDrives:      drives,
				IsSoftwareRAID: false,
			}, nil
		}

		// Wait for md device node to be created by udev/mdev
		time.Sleep(2 * time.Second)

		slog.InfoContext(ctx, "linux software raid array initialized successfully",
			"md_device", mdDevice,
			"raid_level", raidLevelStr,
			"members", raidMemberPartitions,
			"esp_partitions", espPartitions,
		)

		return &StorageLayoutResult{
			TargetDrive:    mdDevice,
			ESPDrives:      espPartitions,
			IsSoftwareRAID: true,
			RAIDDevice:     mdDevice,
		}, nil
	}

	// 2. Standard Single-Disk Layout
	primaryDrive := cfg.TargetDrivePath
	if primaryDrive == "" && len(drives) > 0 {
		primaryDrive = drives[0]
	}

	isLVM := cfg.Storage.LayoutMode == domain.PartitioningLVM || cfg.PartitioningPreset == domain.PartitioningLVM

	slog.InfoContext(ctx, "configuring standard block storage target",
		"target_drive", primaryDrive,
		"is_lvm", isLVM,
	)

	return &StorageLayoutResult{
		TargetDrive:    primaryDrive,
		ESPDrives:      []string{primaryDrive},
		IsSoftwareRAID: false,
		IsLVM:          isLVM,
		LVMVolumeGroup: "vg_system",
	}, nil
}

// resolvePartitionPath returns the partition device node for a disk and partition number.
func resolvePartitionPath(diskPath string, partNum int) string {
	// If path ends in a digit (e.g. /dev/nvme0n1 or /dev/loop0), partition is appended with 'p'
	base := filepath.Base(diskPath)
	lastChar := base[len(base)-1]
	if lastChar >= '0' && lastChar <= '9' {
		return fmt.Sprintf("%sp%d", diskPath, partNum)
	}
	return fmt.Sprintf("%s%d", diskPath, partNum)
}

// InjectMDADMConfig writes the active RAID configuration to the target mounted rootfs.
func InjectMDADMConfig(ctx context.Context, mountPoint string) error {
	mdadmConfDir := filepath.Join(mountPoint, "etc")
	if err := os.MkdirAll(mdadmConfDir, 0755); err != nil {
		return err
	}

	confFile := filepath.Join(mdadmConfDir, "mdadm.conf")
	cmd := exec.CommandContext(ctx, "mdadm", "--detail", "--scan")
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return nil
	}

	existing, _ := os.ReadFile(confFile)
	var content strings.Builder
	content.WriteString(string(existing))
	content.WriteString("\n# Auto-generated by RedWolf Provisioning Engine\n")
	content.WriteString(string(out))
	content.WriteString("\n")

	return os.WriteFile(confFile, []byte(content.String()), 0644)
}
