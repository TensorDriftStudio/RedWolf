package provision

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// StorageLayoutResult represents the prepared storage topology for OS deployment.
type StorageLayoutResult struct {
	TargetDrive    string   // Primary block device to stream raw image into (e.g. /dev/md0 or /dev/nvme0n1)
	ESPDrives      []string // Partitions or disks where the UEFI bootloader must be registered
	MemberESPs     []string // Partition paths of RAID member ESPs (e.g. /dev/sda1, /dev/sdb1)
	IsSoftwareRAID bool
	IsLVM          bool
	RAIDDevice     string
	LVMVolumeGroup string
	RootPartition  string
	RootFSType     string
	BootPartition  string
	BootFSType     string
	ESPPartition   string
	LVMVolumes     []domain.LVMVolumeConfig
	SwapDevice     string
	TargetDisk     string
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
		var memberDisks []string

		for _, disk := range drives {
			realDisk, err := filepath.EvalSymlinks(disk)
			if err != nil {
				realDisk = disk
			}
			memberDisks = append(memberDisks, realDisk)

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

			// Format independent member ESP with FAT32
			_ = exec.CommandContext(ctx, "mkfs.vfat", "-F32", espPart).Run()

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
				TargetDisk:     drives[0],
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
			ESPDrives:      memberDisks,
			MemberESPs:     espPartitions,
			IsSoftwareRAID: true,
			RAIDDevice:     mdDevice,
			TargetDisk:     memberDisks[0],
		}, nil
	}

	// 2. Storage Setup (LVM or Standard Single-Disk)
	primaryDrive := cfg.TargetDrivePath
	if primaryDrive == "" && len(drives) > 0 {
		primaryDrive = drives[0]
	}

	isLVM := cfg.Storage.LayoutMode == domain.PartitioningLVM || cfg.PartitioningPreset == domain.PartitioningLVM
	if isLVM {
		return setupLVMStorage(ctx, primaryDrive, cfg)
	}

	slog.InfoContext(ctx, "configuring standard block storage target",
		"target_drive", primaryDrive,
		"is_lvm", false,
	)

	return &StorageLayoutResult{
		TargetDrive:    primaryDrive,
		ESPDrives:      []string{primaryDrive},
		IsSoftwareRAID: false,
		IsLVM:          false,
		TargetDisk:     primaryDrive,
	}, nil
}

// setupLVMStorage partitions the physical disk with ESP, /boot, and an LVM physical volume,
// creates the volume group 'vg_system', and configures requested logical volumes and swap.
func setupLVMStorage(ctx context.Context, targetDrive string, cfg domain.DeploymentConfig) (*StorageLayoutResult, error) {
	realDisk, err := filepath.EvalSymlinks(targetDrive)
	if err != nil {
		realDisk = targetDrive
	}

	slog.InfoContext(ctx, "initializing LVM volume group storage architecture",
		"target_drive", targetDrive,
		"real_disk", realDisk,
		"preset", cfg.PartitioningPreset,
	)

	// Pre-load device mapper and filesystem kernel modules into the running kernel
	for _, mod := range []string{"dm_mod", "xfs", "ext4"} {
		_ = exec.CommandContext(ctx, "modprobe", mod).Run()
	}

	// Step 1: Deactivate and remove old volume groups/PVs if present
	_ = exec.CommandContext(ctx, "vgchange", "-an", "vg_system").Run()
	_ = exec.CommandContext(ctx, "vgremove", "-y", "-f", "vg_system").Run()
	if err := WipeTargetDisk(ctx, realDisk); err != nil {
		slog.WarnContext(ctx, "pre-wipe warning on LVM target drive", "drive", realDisk, "error", err)
	}

	// Step 2: Create GPT layout:
	// Part 1: ESP (512M) - ef00
	// Part 2: /boot (1024M) - 8300
	// Part 3: LVM PV (remainder) - 8e00
	zapCmd := exec.CommandContext(ctx, "sgdisk", "-Z", realDisk)
	_ = zapCmd.Run()

	partCmd := exec.CommandContext(ctx, "sgdisk",
		"-n", "1:2048:+512M", "-t", "1:ef00", "-c", "1:EFI System Partition",
		"-n", "2:0:+1024M", "-t", "2:8300", "-c", "2:boot",
		"-n", "3:0:0", "-t", "3:8e00", "-c", "3:Linux LVM",
		realDisk,
	)
	if out, err := partCmd.CombinedOutput(); err != nil {
		slog.WarnContext(ctx, "sgdisk partition warning on lvm drive", "drive", realDisk, "error", err, "output", string(out))
	}

	_ = exec.CommandContext(ctx, "partprobe", realDisk).Run()
	_ = exec.CommandContext(ctx, "blockdev", "--rereadpt", realDisk).Run()
	_ = exec.CommandContext(ctx, "mdev", "-s").Run()
	_ = exec.CommandContext(ctx, "udevadm", "settle").Run()
	time.Sleep(1 * time.Second)

	espPart := resolvePartitionPath(realDisk, 1)
	bootPart := resolvePartitionPath(realDisk, 2)
	lvmPart := resolvePartitionPath(realDisk, 3)

	// Ensure LVM physical partition device node is visible in /dev before pvcreate
	for wait := 0; wait < 10; wait++ {
		if _, err := os.Stat(lvmPart); err == nil {
			break
		}
		_ = exec.CommandContext(ctx, "mdev", "-s").Run()
		_ = exec.CommandContext(ctx, "partprobe", realDisk).Run()
		time.Sleep(500 * time.Millisecond)
	}

	// Step 3: Initialize Physical Volume and Volume Group
	pvCmd := exec.CommandContext(ctx, "pvcreate", "-ff", "-y", lvmPart)
	if out, err := pvCmd.CombinedOutput(); err != nil {
		slog.WarnContext(ctx, "pvcreate warning/error", "error", err, "output", string(out))
	}

	vgCmd := exec.CommandContext(ctx, "vgcreate", "-y", "vg_system", lvmPart)
	if out, err := vgCmd.CombinedOutput(); err != nil {
		slog.WarnContext(ctx, "vgcreate returned error; retrying with forced options", "error", err, "output", string(out))
		if out2, err2 := exec.CommandContext(ctx, "vgcreate", "-y", "-ff", "vg_system", lvmPart).CombinedOutput(); err2 != nil {
			return nil, fmt.Errorf("failed creating LVM volume group vg_system on %s: %w (output: %s)", lvmPart, err2, string(out2))
		}
	}

	// Step 4: Format ESP and Boot partitions
	_ = exec.CommandContext(ctx, "mkfs.vfat", "-F32", espPart).Run()
	bootFsType := "xfs"
	if strings.Contains(strings.ToLower(string(cfg.OS)), "debian") {
		bootFsType = "ext4"
		if out, err := exec.CommandContext(ctx, "mkfs.ext4", "-F", bootPart).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("failed formatting boot partition %s with ext4: %w (output: %s)", bootPart, err, string(out))
		}
	} else {
		if out, err := exec.CommandContext(ctx, "mkfs.xfs", "-f", bootPart).CombinedOutput(); err != nil {
			slog.WarnContext(ctx, "xfs formatting warning on boot partition; falling back to ext4", "partition", bootPart, "output", string(out))
			if fbOut, fbErr := exec.CommandContext(ctx, "mkfs.ext4", "-F", bootPart).CombinedOutput(); fbErr != nil {
				return nil, fmt.Errorf("failed formatting boot partition %s: %w (output: %s; ext4 fallback: %s)", bootPart, fbErr, string(out), string(fbOut))
			}
			bootFsType = "ext4"
		}
	}

	// Step 5: Determine and scale Logical Volumes
	vols := cfg.Storage.LVMVolumes
	if len(vols) == 0 {
		vols = []domain.LVMVolumeConfig{
			{Name: "root", MountPoint: "/", SizeGB: 10, FSType: "xfs"},
		}
	}

	// Query VG free capacity
	var freeGB int = 15 // Fallback estimate
	vgsCmd := exec.CommandContext(ctx, "vgs", "--noheadings", "--nosuffix", "--units", "g", "-o", "vg_free", "vg_system")
	if out, err := vgsCmd.Output(); err == nil {
		str := strings.TrimSpace(string(out))
		if idx := strings.Index(str, "."); idx > 0 {
			str = str[:idx]
		}
		if val, err := strconv.Atoi(str); err == nil && val > 0 {
			freeGB = val
		}
	}

	swapGB := cfg.Storage.SwapSizeGB
	totalReq := swapGB
	for _, v := range vols {
		totalReq += v.SizeGB
	}

	// If requested total exceeds available free capacity, scale proportionally
	scale := 1.0
	if totalReq > freeGB && freeGB > 3 {
		scale = float64(freeGB-1) / float64(totalReq)
	}

	// Create Swap LV
	var swapDev string
	if swapGB > 0 {
		scaledSwap := int(float64(swapGB) * scale)
		if scaledSwap < 1 {
			scaledSwap = 1
		}
		lvSwapCmd := exec.CommandContext(ctx, "lvcreate", "-y", "-L", fmt.Sprintf("%dG", scaledSwap), "-n", "swap", "vg_system")
		if out, err := lvSwapCmd.CombinedOutput(); err != nil {
			slog.WarnContext(ctx, "swap lvcreate error; falling back to 1G", "error", err, "output", string(out))
			_ = exec.CommandContext(ctx, "lvcreate", "-y", "-L", "1G", "-n", "swap", "vg_system").Run()
		}
	}

	// Create Data/Root Logical Volumes
	for i, vol := range vols {
		isLast := (i == len(vols)-1)
		scaledSize := int(float64(vol.SizeGB) * scale)
		if scaledSize < 1 {
			scaledSize = 1
		}

		var lvCreate *exec.Cmd
		if isLast {
			// Try 100%FREE for the last volume to utilize remainder, or fallback to fixed size
			lvCreate = exec.CommandContext(ctx, "lvcreate", "-y", "-l", "+100%FREE", "-n", vol.Name, "vg_system")
		} else {
			lvCreate = exec.CommandContext(ctx, "lvcreate", "-y", "-L", fmt.Sprintf("%dG", scaledSize), "-n", vol.Name, "vg_system")
		}

		if out, err := lvCreate.CombinedOutput(); err != nil {
			// If percentage or sizing failed, fallback to 2G fixed
			outFb, errFb := exec.CommandContext(ctx, "lvcreate", "-y", "-L", "2G", "-n", vol.Name, "vg_system").CombinedOutput()
			if errFb != nil {
				slog.ErrorContext(ctx, "failed creating logical volume", "volume", vol.Name, "primary_err", string(out), "fallback_err", string(outFb))
				return nil, fmt.Errorf("failed creating logical volume %s: %w (output: %s)", vol.Name, errFb, string(outFb))
			}
		}
	}

	// Activate volume group and instantiate device nodes
	_ = exec.CommandContext(ctx, "vgchange", "-ay", "vg_system").Run()
	_ = exec.CommandContext(ctx, "vgmknodes").Run()
	_ = exec.CommandContext(ctx, "mdev", "-s").Run()
	_ = exec.CommandContext(ctx, "udevadm", "settle").Run()
	time.Sleep(1 * time.Second)

	// Format swap volume
	if swapGB > 0 {
		swapDevNode := resolveLVMDeviceNode("vg_system", "swap")
		if out, err := exec.CommandContext(ctx, "mkswap", "-f", swapDevNode).CombinedOutput(); err != nil {
			slog.WarnContext(ctx, "mkswap warning", "device", swapDevNode, "error", err, "output", string(out))
		}
		swapDev = swapDevNode
	}

	// Format data/root logical volumes
	var rootDevNode string
	var rootFSType string

	for i, vol := range vols {
		devNode := resolveLVMDeviceNode("vg_system", vol.Name)
		fsType := strings.ToLower(strings.TrimSpace(vol.FSType))
		if fsType == "" {
			if strings.Contains(strings.ToLower(string(cfg.OS)), "debian") {
				fsType = "ext4"
			} else {
				fsType = "xfs"
			}
		}

		slog.InfoContext(ctx, "formatting logical volume",
			"volume", vol.Name,
			"device", devNode,
			"fstype", fsType,
		)

		var mkfsCmd *exec.Cmd
		if fsType == "ext4" {
			mkfsCmd = exec.CommandContext(ctx, "mkfs.ext4", "-F", devNode)
		} else {
			mkfsCmd = exec.CommandContext(ctx, "mkfs.xfs", "-f", devNode)
		}

		if out, err := mkfsCmd.CombinedOutput(); err != nil {
			slog.WarnContext(ctx, "mkfs failed on logical volume; attempting ext4 fallback",
				"volume", vol.Name,
				"fstype", fsType,
				"error", err,
				"output", string(out),
			)
			if fbOut, fbErr := exec.CommandContext(ctx, "mkfs.ext4", "-F", devNode).CombinedOutput(); fbErr != nil {
				return nil, fmt.Errorf("failed formatting logical volume %s (%s) with %s: %w (output: %s; ext4 fallback: %s)",
					vol.Name, devNode, fsType, err, string(out), string(fbOut))
			}
			vols[i].FSType = "ext4"
			fsType = "ext4"
		}

		if vol.MountPoint == "/" || vol.Name == "root" {
			rootDevNode = devNode
			rootFSType = fsType
		}
	}

	if rootDevNode == "" {
		rootDevNode = resolveLVMDeviceNode("vg_system", "root")
		if rootFSType == "" {
			rootFSType = "xfs"
		}
	}

	return &StorageLayoutResult{
		TargetDrive:    rootDevNode,
		ESPDrives:      []string{espPart},
		IsSoftwareRAID: false,
		IsLVM:          true,
		LVMVolumeGroup: "vg_system",
		RootPartition:  rootDevNode,
		RootFSType:     rootFSType,
		BootPartition:  bootPart,
		BootFSType:     bootFsType,
		ESPPartition:   espPart,
		LVMVolumes:     vols,
		SwapDevice:     swapDev,
		TargetDisk:     realDisk,
	}, nil
}

// resolveLVMDeviceNode returns the active block device path for an LVM volume,
// checking standard symlink (/dev/<vg>/<lv>) and canonical device-mapper node (/dev/mapper/<vg>-<lv>).
// It also ensures the /dev/<vg>/<lv> symlink exists even in minimal udev-less environments.
func resolveLVMDeviceNode(vgName, lvName string) string {
	mapperPath := fmt.Sprintf("/dev/mapper/%s-%s",
		strings.ReplaceAll(vgName, "-", "--"),
		strings.ReplaceAll(lvName, "-", "--"),
	)
	symlinkDir := fmt.Sprintf("/dev/%s", vgName)
	symlinkPath := fmt.Sprintf("%s/%s", symlinkDir, lvName)

	if _, err := os.Stat(symlinkPath); err == nil {
		return symlinkPath
	}
	if _, err := os.Stat(mapperPath); err == nil {
		_ = os.MkdirAll(symlinkDir, 0755)
		_ = os.Symlink(mapperPath, symlinkPath)
		return mapperPath
	}

	// Fallback to mapperPath as canonical
	return mapperPath
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
// It populates both /etc/mdadm.conf (RHEL/AlmaLinux standard) and /etc/mdadm/mdadm.conf (Debian standard).
func InjectMDADMConfig(ctx context.Context, mountPoint string) error {
	cmd := exec.CommandContext(ctx, "mdadm", "--detail", "--scan")
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return nil
	}

	confDirs := []string{
		filepath.Join(mountPoint, "etc"),
		filepath.Join(mountPoint, "etc", "mdadm"),
	}
	confFiles := []string{
		filepath.Join(mountPoint, "etc", "mdadm.conf"),
		filepath.Join(mountPoint, "etc", "mdadm", "mdadm.conf"),
	}

	for i, confFile := range confFiles {
		if err := os.MkdirAll(confDirs[i], 0755); err != nil {
			continue
		}
		existing, _ := os.ReadFile(confFile)
		var content strings.Builder
		content.WriteString(string(existing))
		content.WriteString("\n# Auto-generated by RedWolf Provisioning Engine\n")
		content.WriteString(string(out))
		content.WriteString("\n")
		_ = os.WriteFile(confFile, []byte(content.String()), 0644)
	}

	return nil
}

