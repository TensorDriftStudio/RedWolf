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
	TargetDrive    string   // Primary block device or root LV (e.g. /dev/mapper/vg_system-root, /dev/md1, or /dev/nvme0n1)
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

	isRAID := (cfg.Storage.RAIDLevel == domain.RAIDLevel1 && len(drives) >= 2) ||
		(cfg.Storage.RAIDLevel == domain.RAIDLevel0 && len(drives) >= 2) ||
		(cfg.Storage.RAIDLevel == domain.RAIDLevel10 && len(drives) >= 4) ||
		(cfg.PartitioningPreset == domain.PartitioningRAID1 && len(drives) >= 2)

	isLVM := cfg.Storage.LayoutMode == domain.PartitioningLVM || cfg.PartitioningPreset == domain.PartitioningLVM

	// 1. Software RAID Execution (with or without LVM)
	if isRAID {
		return setupSoftwareRAID(ctx, drives, cfg, isLVM)
	}

	primaryDrive := cfg.TargetDrivePath
	if primaryDrive == "" && len(drives) > 0 {
		primaryDrive = drives[0]
	}

	// 2. Single-Disk LVM Setup
	if isLVM {
		return setupSingleDiskLVM(ctx, primaryDrive, cfg)
	}

	// 3. Single-Disk Standard Setup
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

// setupSoftwareRAID configures multi-disk Linux mdraid arrays for /boot and root/LVM.
func setupSoftwareRAID(ctx context.Context, drives []string, cfg domain.DeploymentConfig, isLVM bool) (*StorageLayoutResult, error) {
	slog.InfoContext(ctx, "configuring multi-disk Linux Software RAID array",
		"raid_level", cfg.Storage.RAIDLevel,
		"preset", cfg.PartitioningPreset,
		"target_drives", drives,
		"is_lvm", isLVM,
	)

	// Pre-load required RAID, LVM, and filesystem kernel drivers
	for _, mod := range []string{"dm_mod", "md_mod", "raid0", "raid1", "raid10", "linear", "xfs", "ext4", "vfat"} {
		_ = exec.CommandContext(ctx, "modprobe", mod).Run()
	}

	raidLevelStr := "1"
	if cfg.Storage.RAIDLevel == domain.RAIDLevel0 {
		raidLevelStr = "0"
	} else if cfg.Storage.RAIDLevel == domain.RAIDLevel10 {
		raidLevelStr = "10"
	}

	// Stop any existing volume groups or md arrays
	_ = exec.CommandContext(ctx, "vgchange", "-an", "vg_system").Run()
	_ = exec.CommandContext(ctx, "vgremove", "-y", "-f", "vg_system").Run()
	_ = exec.CommandContext(ctx, "mdadm", "--stop", "/dev/md0").Run()
	_ = exec.CommandContext(ctx, "mdadm", "--stop", "/dev/md1").Run()
	_ = exec.CommandContext(ctx, "mdadm", "--stop", "--scan").Run()

	var espPartitions []string
	var bootRaidMembers []string
	var dataRaidMembers []string
	var memberDisks []string

	for _, disk := range drives {
		realDisk, err := filepath.EvalSymlinks(disk)
		if err != nil {
			realDisk = disk
		}
		memberDisks = append(memberDisks, realDisk)

		// Pre-wipe disk signatures and boundary structures
		if err := WipeTargetDisk(ctx, realDisk); err != nil {
			slog.WarnContext(ctx, "pre-wipe warning on raid drive", "drive", realDisk, "error", err)
		}

		// Partition layout (Universal BIOS + UEFI dual-boot architecture):
		// Part 1: BIOS Boot Partition (1 MiB) - ef02
		// Part 2: ESP (1024 MiB) - ef00
		// Part 3: /boot RAID (1024 MiB) - fd00
		// Part 4: Data/LVM RAID (Remainder) - fd00
		_ = exec.CommandContext(ctx, "sgdisk", "-Z", realDisk).Run()
		partCmd := exec.CommandContext(ctx, "sgdisk",
			"-n", "1:2048:+1M", "-t", "1:ef02", "-c", "1:BIOS Boot Partition",
			"-n", "2:0:+1024M", "-t", "2:ef00", "-c", "2:EFI System Partition",
			"-n", "3:0:+1024M", "-t", "3:fd00", "-c", "3:Linux Software RAID boot",
			"-n", "4:0:0", "-t", "4:fd00", "-c", "4:Linux Software RAID data",
			"-A", "1:set:2",
			realDisk,
		)
		if out, err := partCmd.CombinedOutput(); err != nil {
			slog.WarnContext(ctx, "sgdisk partition warning on raid drive, trying parted fallback", "drive", realDisk, "error", err, "output", string(out))
			_ = exec.CommandContext(ctx, "parted", "-s", realDisk, "mklabel", "gpt").Run()
			partedCmd := exec.CommandContext(ctx, "parted", "-s", "-a", "optimal", realDisk,
				"mkpart", "bios_grub", "1MiB", "2MiB",
				"set", "1", "bios_grub", "on",
				"mkpart", "ESP", "fat32", "2MiB", "1026MiB",
				"set", "2", "esp", "on",
				"set", "2", "boot", "on",
				"mkpart", "boot", "ext4", "1026MiB", "2050MiB",
				"set", "3", "raid", "on",
				"mkpart", "data", "ext4", "2050MiB", "100%",
				"set", "4", "raid", "on",
			)
			if pOut, pErr := partedCmd.CombinedOutput(); pErr != nil {
				return nil, fmt.Errorf("failed creating GPT partitions on RAID drive %s (sgdisk: %v; parted: %w output: %s)", realDisk, err, pErr, string(pOut))
			}
		}

		_ = exec.CommandContext(ctx, "parted", "-s", realDisk, "disk_set", "pmbr_boot", "on").Run()
		settlePartitions(ctx, realDisk)

		biosPart := resolvePartitionPath(realDisk, 1)
		espPart := resolvePartitionPath(realDisk, 2)
		bootPart := resolvePartitionPath(realDisk, 3)
		dataPart := resolvePartitionPath(realDisk, 4)

		if err := waitForDevice(ctx, biosPart, 5*time.Second); err != nil {
			slog.DebugContext(ctx, "waiting for bios boot partition", "partition", biosPart, "error", err)
		}
		if err := waitForDevice(ctx, espPart, 5*time.Second); err != nil {
			return nil, fmt.Errorf("failed waiting for ESP partition %s: %w", espPart, err)
		}
		if err := waitForDevice(ctx, bootPart, 5*time.Second); err != nil {
			return nil, fmt.Errorf("failed waiting for boot partition %s: %w", bootPart, err)
		}
		if err := waitForDevice(ctx, dataPart, 5*time.Second); err != nil {
			return nil, fmt.Errorf("failed waiting for data partition %s: %w", dataPart, err)
		}

		// Format independent member ESP with FAT32
		if out, err := exec.CommandContext(ctx, "mkfs.vfat", "-F32", espPart).CombinedOutput(); err != nil {
			slog.WarnContext(ctx, "mkfs.vfat warning on esp partition", "partition", espPart, "output", string(out), "error", err)
		}

		// Clear leftover superblocks on member partitions
		_ = exec.CommandContext(ctx, "wipefs", "-a", "-f", bootPart).Run()
		_ = exec.CommandContext(ctx, "wipefs", "-a", "-f", dataPart).Run()
		_ = exec.CommandContext(ctx, "mdadm", "--zero-superblock", "--force", bootPart).Run()
		_ = exec.CommandContext(ctx, "mdadm", "--zero-superblock", "--force", dataPart).Run()

		espPartitions = append(espPartitions, espPart)
		bootRaidMembers = append(bootRaidMembers, bootPart)
		dataRaidMembers = append(dataRaidMembers, dataPart)
	}

	// Step 1: Create /boot RAID 1 array on /dev/md0
	// Using --metadata=1.0 puts superblock at end of partition, enabling bootloaders to read directly
	bootMD := "/dev/md0"
	mdadmBootArgs := []string{
		"--create", bootMD,
		"--level=1",
		fmt.Sprintf("--raid-devices=%d", len(bootRaidMembers)),
		"--metadata=1.0",
		"--force",
		"--run",
	}
	mdadmBootArgs = append(mdadmBootArgs, bootRaidMembers...)
	if out, err := exec.CommandContext(ctx, "mdadm", mdadmBootArgs...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed creating boot RAID array %s: %w (output: %s)", bootMD, err, string(out))
	}
	_ = waitForDevice(ctx, bootMD, 5*time.Second)

	bootFsType := "ext4"
	if out, err := exec.CommandContext(ctx, "mkfs.ext4", "-F", "-L", "boot", bootMD).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed formatting boot RAID %s with ext4: %w (output: %s)", bootMD, err, string(out))
	}

	// Step 2: Create Data/Root RAID array on /dev/md1
	dataMD := "/dev/md1"
	mdadmDataArgs := []string{
		"--create", dataMD,
		fmt.Sprintf("--level=%s", raidLevelStr),
		fmt.Sprintf("--raid-devices=%d", len(dataRaidMembers)),
		"--metadata=1.2",
		"--force",
		"--run",
	}
	mdadmDataArgs = append(mdadmDataArgs, dataRaidMembers...)
	if out, err := exec.CommandContext(ctx, "mdadm", mdadmDataArgs...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed creating data RAID array %s: %w (output: %s)", dataMD, err, string(out))
	}
	_ = waitForDevice(ctx, dataMD, 5*time.Second)

	slog.InfoContext(ctx, "software raid arrays initialized successfully",
		"boot_md", bootMD,
		"data_md", dataMD,
		"level", raidLevelStr,
		"is_lvm", isLVM,
	)

	// Step 3: If LVM is requested, build Volume Group vg_system on top of dataMD
	if isLVM {
		lvmRes, err := buildLVMOnBlockDevice(ctx, dataMD, cfg)
		if err != nil {
			return nil, err
		}
		lvmRes.ESPDrives = memberDisks
		lvmRes.MemberESPs = espPartitions
		lvmRes.IsSoftwareRAID = true
		lvmRes.RAIDDevice = dataMD
		lvmRes.BootPartition = bootMD
		lvmRes.BootFSType = bootFsType
		lvmRes.ESPPartition = espPartitions[0]
		lvmRes.TargetDisk = memberDisks[0]
		return lvmRes, nil
	}

	// Step 4: Standard Software RAID without LVM - format dataMD as root filesystem
	rootFsType := "xfs"
	if strings.Contains(strings.ToLower(string(cfg.OS)), "debian") {
		rootFsType = "ext4"
		if out, err := exec.CommandContext(ctx, "mkfs.ext4", "-F", dataMD).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("failed formatting root RAID %s with ext4: %w (output: %s)", dataMD, err, string(out))
		}
	} else {
		if out, err := exec.CommandContext(ctx, "mkfs.xfs", "-f", dataMD).CombinedOutput(); err != nil {
			slog.WarnContext(ctx, "xfs formatting warning on root RAID; attempting ext4 fallback", "output", string(out))
			if fbOut, fbErr := exec.CommandContext(ctx, "mkfs.ext4", "-F", dataMD).CombinedOutput(); fbErr != nil {
				return nil, fmt.Errorf("failed formatting root RAID %s: %w (output: %s; ext4 fallback: %s)", dataMD, fbErr, string(out), string(fbOut))
			}
			rootFsType = "ext4"
		}
	}

	return &StorageLayoutResult{
		TargetDrive:    dataMD,
		ESPDrives:      memberDisks,
		MemberESPs:     espPartitions,
		IsSoftwareRAID: true,
		IsLVM:          false,
		RAIDDevice:     dataMD,
		RootPartition:  dataMD,
		RootFSType:     rootFsType,
		BootPartition:  bootMD,
		BootFSType:     bootFsType,
		ESPPartition:   espPartitions[0],
		TargetDisk:     memberDisks[0],
	}, nil
}

// setupSingleDiskLVM partitions a single physical disk with ESP, /boot, and an LVM PV.
func setupSingleDiskLVM(ctx context.Context, targetDrive string, cfg domain.DeploymentConfig) (*StorageLayoutResult, error) {
	realDisk, err := filepath.EvalSymlinks(targetDrive)
	if err != nil {
		realDisk = targetDrive
	}

	slog.InfoContext(ctx, "initializing single-disk LVM volume group storage architecture",
		"target_drive", targetDrive,
		"real_disk", realDisk,
		"preset", cfg.PartitioningPreset,
	)

	for _, mod := range []string{"dm_mod", "xfs", "ext4", "vfat"} {
		_ = exec.CommandContext(ctx, "modprobe", mod).Run()
	}

	_ = exec.CommandContext(ctx, "vgchange", "-an", "vg_system").Run()
	_ = exec.CommandContext(ctx, "vgremove", "-y", "-f", "vg_system").Run()
	if err := WipeTargetDisk(ctx, realDisk); err != nil {
		slog.WarnContext(ctx, "pre-wipe warning on LVM target drive", "drive", realDisk, "error", err)
	}

	// Universal Hybrid Partition layout (BIOS + UEFI dual-boot architecture):
	// Part 1: BIOS Boot (1M) - ef02
	// Part 2: ESP (1024M) - ef00
	// Part 3: /boot (1024M) - 8300 (ext4)
	// Part 4: Linux LVM (remainder) - 8e00
	_ = exec.CommandContext(ctx, "sgdisk", "-Z", realDisk).Run()
	partCmd := exec.CommandContext(ctx, "sgdisk",
		"-n", "1:2048:+1M", "-t", "1:ef02", "-c", "1:BIOS Boot Partition",
		"-n", "2:0:+1024M", "-t", "2:ef00", "-c", "2:EFI System Partition",
		"-n", "3:0:+1024M", "-t", "3:8300", "-c", "3:boot",
		"-n", "4:0:0", "-t", "4:8e00", "-c", "4:Linux LVM",
		"-A", "1:set:2",
		realDisk,
	)
	if out, err := partCmd.CombinedOutput(); err != nil {
		slog.WarnContext(ctx, "sgdisk partition warning on lvm drive, trying parted fallback", "drive", realDisk, "error", err, "output", string(out))
		_ = exec.CommandContext(ctx, "parted", "-s", realDisk, "mklabel", "gpt").Run()
		partedCmd := exec.CommandContext(ctx, "parted", "-s", "-a", "optimal", realDisk,
			"mkpart", "bios_grub", "1MiB", "2MiB",
			"set", "1", "bios_grub", "on",
			"mkpart", "ESP", "fat32", "2MiB", "1026MiB",
			"set", "2", "esp", "on",
			"set", "2", "boot", "on",
			"mkpart", "boot", "ext4", "1026MiB", "2050MiB",
			"mkpart", "lvm", "2050MiB", "100%",
			"set", "4", "lvm", "on",
		)
		if pOut, pErr := partedCmd.CombinedOutput(); pErr != nil {
			return nil, fmt.Errorf("failed creating GPT partitions on LVM drive %s (sgdisk: %v; parted: %w output: %s)", realDisk, err, pErr, string(pOut))
		}
	}

	_ = exec.CommandContext(ctx, "parted", "-s", realDisk, "disk_set", "pmbr_boot", "on").Run()
	settlePartitions(ctx, realDisk)

	biosPart := resolvePartitionPath(realDisk, 1)
	espPart := resolvePartitionPath(realDisk, 2)
	bootPart := resolvePartitionPath(realDisk, 3)
	lvmPart := resolvePartitionPath(realDisk, 4)

	if err := waitForDevice(ctx, biosPart, 5*time.Second); err != nil {
		slog.DebugContext(ctx, "waiting for bios boot partition", "partition", biosPart, "error", err)
	}
	if err := waitForDevice(ctx, espPart, 5*time.Second); err != nil {
		return nil, fmt.Errorf("failed waiting for ESP partition %s: %w", espPart, err)
	}
	if err := waitForDevice(ctx, bootPart, 5*time.Second); err != nil {
		return nil, fmt.Errorf("failed waiting for boot partition %s: %w", bootPart, err)
	}
	if err := waitForDevice(ctx, lvmPart, 5*time.Second); err != nil {
		return nil, fmt.Errorf("failed waiting for LVM partition %s: %w", lvmPart, err)
	}

	// Format ESP and Boot partitions
	_ = exec.CommandContext(ctx, "mkfs.vfat", "-F32", espPart).Run()
	bootFsType := "ext4"
	if out, err := exec.CommandContext(ctx, "mkfs.ext4", "-F", "-L", "boot", bootPart).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed formatting boot partition %s with ext4: %w (output: %s)", bootPart, err, string(out))
	}

	lvmRes, err := buildLVMOnBlockDevice(ctx, lvmPart, cfg)
	if err != nil {
		return nil, err
	}
	lvmRes.ESPDrives = []string{realDisk}
	lvmRes.IsSoftwareRAID = false
	lvmRes.BootPartition = bootPart
	lvmRes.BootFSType = bootFsType
	lvmRes.ESPPartition = espPart
	lvmRes.TargetDisk = realDisk

	return lvmRes, nil
}

// buildLVMOnBlockDevice initializes pvcreate, vgcreate, creates logical volumes and formats filesystems.
func buildLVMOnBlockDevice(ctx context.Context, pvDevice string, cfg domain.DeploymentConfig) (*StorageLayoutResult, error) {
	// Initialize PV and VG
	pvCmd := exec.CommandContext(ctx, "pvcreate", "-ff", "-y", pvDevice)
	if out, err := pvCmd.CombinedOutput(); err != nil {
		slog.WarnContext(ctx, "pvcreate warning/error", "error", err, "output", string(out))
	}

	vgCmd := exec.CommandContext(ctx, "vgcreate", "-y", "-ff", "vg_system", pvDevice)
	if out, err := vgCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed creating LVM volume group vg_system on %s: %w (output: %s)", pvDevice, err, string(out))
	}

	vols := cfg.Storage.LVMVolumes
	if len(vols) == 0 {
		vols = []domain.LVMVolumeConfig{
			{Name: "root", MountPoint: "/", SizeGB: 10, FSType: "xfs"},
		}
	}

	// Query VG free capacity
	var freeGB int = 15
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

	// Create Logical Volumes
	for i, vol := range vols {
		isLast := (i == len(vols)-1)
		scaledSize := int(float64(vol.SizeGB) * scale)
		if scaledSize < 1 {
			scaledSize = 1
		}

		var lvCreate *exec.Cmd
		if isLast {
			lvCreate = exec.CommandContext(ctx, "lvcreate", "-y", "-l", "+100%FREE", "-n", vol.Name, "vg_system")
		} else {
			lvCreate = exec.CommandContext(ctx, "lvcreate", "-y", "-L", fmt.Sprintf("%dG", scaledSize), "-n", vol.Name, "vg_system")
		}

		if out, err := lvCreate.CombinedOutput(); err != nil {
			outFb, errFb := exec.CommandContext(ctx, "lvcreate", "-y", "-L", "2G", "-n", vol.Name, "vg_system").CombinedOutput()
			if errFb != nil {
				return nil, fmt.Errorf("failed creating logical volume %s: %w (output: %s; fallback: %s)", vol.Name, errFb, string(out), string(outFb))
			}
		}
	}

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
		IsLVM:          true,
		LVMVolumeGroup: "vg_system",
		RootPartition:  rootDevNode,
		RootFSType:     rootFSType,
		LVMVolumes:     vols,
		SwapDevice:     swapDev,
	}, nil
}

// resolveLVMDeviceNode returns the active block device path for an LVM volume,
// checking standard symlink (/dev/<vg>/<lv>) and canonical device-mapper node (/dev/mapper/<vg>-<lv>).
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

	return mapperPath
}

// resolvePartitionPath returns the partition device node for a disk and partition number.
func resolvePartitionPath(diskPath string, partNum int) string {
	// 1. If path is a by-id or by-path symlink that exists with -part suffix
	if strings.Contains(diskPath, "/by-id/") || strings.Contains(diskPath, "/by-path/") {
		cand := fmt.Sprintf("%s-part%d", diskPath, partNum)
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}

	// 2. Evaluate symlinks if possible (e.g. /dev/disk/by-id/virtio-xxx -> /dev/vda)
	target := diskPath
	if realDev, err := filepath.EvalSymlinks(diskPath); err == nil && realDev != "" {
		target = realDev
	}

	base := filepath.Base(target)
	lastChar := base[len(base)-1]
	if lastChar >= '0' && lastChar <= '9' {
		return fmt.Sprintf("%sp%d", target, partNum)
	}
	return fmt.Sprintf("%s%d", target, partNum)
}

// settlePartitions refreshes partition tables in the running kernel and udev/mdev.
func settlePartitions(ctx context.Context, disk string) {
	_ = exec.CommandContext(ctx, "partprobe", disk).Run()
	_ = exec.CommandContext(ctx, "blockdev", "--rereadpt", disk).Run()
	_ = exec.CommandContext(ctx, "mdev", "-s").Run()
	_ = exec.CommandContext(ctx, "udevadm", "settle").Run()
	time.Sleep(500 * time.Millisecond)
}

// waitForDevice polls until a device node exists in /dev or timeout expires.
func waitForDevice(ctx context.Context, devPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(devPath); err == nil {
			return nil
		}
		_ = exec.CommandContext(ctx, "mdev", "-s").Run()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	if _, err := os.Stat(devPath); err == nil {
		return nil
	}
	return fmt.Errorf("device %s did not appear within %v", devPath, timeout)
}

// InjectMDADMConfig writes the active RAID configuration to the target mounted rootfs.
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
		content.WriteString("DEVICE partitions\n")
		content.WriteString(string(out))
		content.WriteString("\n")
		_ = os.WriteFile(confFile, []byte(content.String()), 0644)
	}

	return nil
}
