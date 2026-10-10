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
	BootRAIDUUID   string
	DataRAIDUUID   string
}

// SetupStorageArchitecture provisions disks according to standard, software RAID, or LVM configurations.
func SetupStorageArchitecture(ctx context.Context, cfg domain.DeploymentConfig) (*StorageLayoutResult, error) {
	rawDrives := cfg.Storage.TargetDrives
	if len(rawDrives) == 0 && cfg.TargetDrivePath != "" {
		rawDrives = []string{cfg.TargetDrivePath}
	}

	seenDrives := make(map[string]bool)
	var drives []string
	for _, d := range rawDrives {
		trimmed := strings.TrimSpace(d)
		if trimmed == "" {
			continue
		}
		realDev, err := filepath.EvalSymlinks(trimmed)
		if err != nil || realDev == "" {
			realDev = trimmed
		}
		if !seenDrives[realDev] {
			seenDrives[realDev] = true
			drives = append(drives, trimmed)
		}
	}

	isRAID := (cfg.Storage.RAIDLevel == domain.RAIDLevel1 && len(drives) >= 2) ||
		(cfg.Storage.RAIDLevel == domain.RAIDLevel0 && len(drives) >= 2) ||
		(cfg.Storage.RAIDLevel == domain.RAIDLevel10 && len(drives) >= 4) ||
		(cfg.PartitioningPreset == domain.PartitioningRAID1 && len(drives) >= 2)

	isLVM := cfg.Storage.LayoutMode == domain.PartitioningLVM || cfg.PartitioningPreset == domain.PartitioningLVM

	if isRAID {
		return setupSoftwareRAID(ctx, drives, cfg, isLVM)
	}

	primaryDrive := cfg.TargetDrivePath
	if primaryDrive == "" && len(drives) > 0 {
		primaryDrive = drives[0]
	}

	if isLVM {
		return setupSingleDiskLVM(ctx, primaryDrive, cfg)
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

// setupSoftwareRAID configures multi-disk Linux mdraid arrays for /boot and root/LVM.
func setupSoftwareRAID(ctx context.Context, drives []string, cfg domain.DeploymentConfig, isLVM bool) (*StorageLayoutResult, error) {
	slog.InfoContext(ctx, "configuring multi-disk Linux Software RAID array",
		"raid_level", cfg.Storage.RAIDLevel,
		"preset", cfg.PartitioningPreset,
		"target_drives", drives,
		"is_lvm", isLVM,
	)

	for _, mod := range []string{"dm_mod", "md_mod", "raid0", "raid1", "raid10", "linear", "xfs", "ext4", "vfat"} {
		_ = exec.CommandContext(ctx, "modprobe", mod).Run()
	}

	raidLevelStr := "1"
	if cfg.Storage.RAIDLevel == domain.RAIDLevel0 {
		raidLevelStr = "0"
	} else if cfg.Storage.RAIDLevel == domain.RAIDLevel10 {
		raidLevelStr = "10"
	}

	_ = exec.CommandContext(ctx, "umount", "-R", "/mnt").Run()
	_ = exec.CommandContext(ctx, "swapoff", "-a").Run()
	_ = exec.CommandContext(ctx, "vgchange", "-an", "vg_system").Run()
	_ = exec.CommandContext(ctx, "vgremove", "-y", "-f", "vg_system").Run()
	_ = exec.CommandContext(ctx, "mdadm", "--stop", "/dev/md0").Run()
	_ = exec.CommandContext(ctx, "mdadm", "--stop", "/dev/md1").Run()
	_ = exec.CommandContext(ctx, "mdadm", "--stop", "/dev/md127").Run()
	_ = exec.CommandContext(ctx, "mdadm", "--stop", "/dev/md126").Run()
	_ = exec.CommandContext(ctx, "mdadm", "--stop", "/dev/md/0").Run()
	_ = exec.CommandContext(ctx, "mdadm", "--stop", "/dev/md/1").Run()
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

		_, espPart, bootPart, dataPart, err := partitionUniversalDisk(ctx, realDisk, "fd00", "fd00", "Linux Software RAID boot", "Linux Software RAID data")
		if err != nil {
			return nil, err
		}

		_ = exec.CommandContext(ctx, "wipefs", "-a", "-f", bootPart).Run()
		_ = exec.CommandContext(ctx, "wipefs", "-a", "-f", dataPart).Run()
		_ = exec.CommandContext(ctx, "mdadm", "--zero-superblock", "--force", bootPart).Run()
		_ = exec.CommandContext(ctx, "mdadm", "--zero-superblock", "--force", dataPart).Run()

		espPartitions = append(espPartitions, espPart)
		bootRaidMembers = append(bootRaidMembers, bootPart)
		dataRaidMembers = append(dataRaidMembers, dataPart)
	}

	bootMD := "/dev/md0"
	mdadmBootArgs := []string{
		"--create", bootMD,
		"--level=1",
		fmt.Sprintf("--raid-devices=%d", len(bootRaidMembers)),
		"--metadata=1.0",
		"--homehost=any",
		"--force",
		"--run",
	}
	mdadmBootArgs = append(mdadmBootArgs, bootRaidMembers...)
	if out, err := exec.CommandContext(ctx, "mdadm", mdadmBootArgs...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed creating boot RAID array %s: %w (output: %s)", bootMD, err, string(out))
	}
	_ = waitForDevice(ctx, bootMD, 5*time.Second)
	ensureMDDeviceNode(bootMD, 0)

	bootFsType := "ext4"
	if out, err := exec.CommandContext(ctx, "mkfs.ext4", "-F", "-L", "boot", bootMD).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed formatting boot RAID %s with ext4: %w (output: %s)", bootMD, err, string(out))
	}

	dataMD := "/dev/md1"
	mdadmDataArgs := []string{
		"--create", dataMD,
		fmt.Sprintf("--level=%s", raidLevelStr),
		fmt.Sprintf("--raid-devices=%d", len(dataRaidMembers)),
		"--metadata=1.2",
		"--homehost=any",
		"--force",
		"--run",
	}
	mdadmDataArgs = append(mdadmDataArgs, dataRaidMembers...)
	if out, err := exec.CommandContext(ctx, "mdadm", mdadmDataArgs...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed creating data RAID array %s: %w (output: %s)", dataMD, err, string(out))
	}
	_ = waitForDevice(ctx, dataMD, 5*time.Second)
	ensureMDDeviceNode(dataMD, 1)

	bootRaidUUID := getMDArrayUUID(ctx, bootMD)
	dataRaidUUID := getMDArrayUUID(ctx, dataMD)

	slog.InfoContext(ctx, "software raid arrays initialized successfully",
		"boot_md", bootMD,
		"boot_md_uuid", bootRaidUUID,
		"data_md", dataMD,
		"data_md_uuid", dataRaidUUID,
		"level", raidLevelStr,
		"is_lvm", isLVM,
	)

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
		lvmRes.BootRAIDUUID = bootRaidUUID
		lvmRes.DataRAIDUUID = dataRaidUUID
		return lvmRes, nil
	}

	rootFsType := "xfs"
	isDebianOrUbuntu := strings.Contains(strings.ToLower(string(cfg.OS)), "debian") || strings.Contains(strings.ToLower(string(cfg.OS)), "ubuntu")
	if isDebianOrUbuntu {
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
		BootRAIDUUID:   bootRaidUUID,
		DataRAIDUUID:   dataRaidUUID,
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

	_, espPart, bootPart, lvmPart, err := partitionUniversalDisk(ctx, realDisk, "8300", "8e00", "boot", "Linux LVM")
	if err != nil {
		return nil, err
	}

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

// partitionUniversalDisk provisions a standard 4-partition dual BIOS/UEFI layout on target disk:
// Part 1: BIOS Boot (1 MiB, ef02)
// Part 2: EFI System Partition (1024 MiB, ef00)
// Part 3: Boot partition (1024 MiB, part3Type)
// Part 4: Data partition (remainder, part4Type)
func partitionUniversalDisk(ctx context.Context, disk, part3Type, part4Type, part3Name, part4Name string) (string, string, string, string, error) {
	if err := WipeTargetDisk(ctx, disk); err != nil {
		slog.WarnContext(ctx, "pre-wipe warning on target drive", "drive", disk, "error", err)
	}

	_ = exec.CommandContext(ctx, "sgdisk", "-Z", disk).Run()
	partCmd := exec.CommandContext(ctx, "sgdisk",
		"-n", "1:2048:+1M", "-t", "1:ef02", "-c", "1:BIOS Boot Partition",
		"-n", "2:0:+1024M", "-t", "2:ef00", "-c", "2:EFI System Partition",
		"-n", "3:0:+1024M", "-t", fmt.Sprintf("3:%s", part3Type), "-c", fmt.Sprintf("3:%s", part3Name),
		"-n", "4:0:0", "-t", fmt.Sprintf("4:%s", part4Type), "-c", fmt.Sprintf("4:%s", part4Name),
		"-A", "1:set:2",
		disk,
	)
	if out, err := partCmd.CombinedOutput(); err != nil {
		slog.WarnContext(ctx, "sgdisk failed, using parted fallback", "drive", disk, "error", err, "output", string(out))
		if fbErr := executePartedFallback(ctx, disk, part3Type, part4Type, part3Name, part4Name); fbErr != nil {
			return "", "", "", "", fmt.Errorf("failed partitioning drive %s: %w", disk, fbErr)
		}
	}

	_ = exec.CommandContext(ctx, "parted", "-s", disk, "disk_set", "pmbr_boot", "on").Run()
	settlePartitions(ctx, disk)

	biosPart := resolvePartitionPath(disk, 1)
	espPart := resolvePartitionPath(disk, 2)
	bootPart := resolvePartitionPath(disk, 3)
	dataPart := resolvePartitionPath(disk, 4)

	_ = waitForDevice(ctx, biosPart, 5*time.Second)
	if err := waitForDevice(ctx, espPart, 5*time.Second); err != nil {
		return "", "", "", "", fmt.Errorf("failed waiting for ESP partition %s: %w", espPart, err)
	}
	if err := waitForDevice(ctx, bootPart, 5*time.Second); err != nil {
		return "", "", "", "", fmt.Errorf("failed waiting for boot partition %s: %w", bootPart, err)
	}
	if err := waitForDevice(ctx, dataPart, 5*time.Second); err != nil {
		return "", "", "", "", fmt.Errorf("failed waiting for data partition %s: %w", dataPart, err)
	}

	if out, err := exec.CommandContext(ctx, "mkfs.vfat", "-F32", espPart).CombinedOutput(); err != nil {
		slog.WarnContext(ctx, "mkfs.vfat warning on esp partition", "partition", espPart, "output", string(out), "error", err)
	}

	return biosPart, espPart, bootPart, dataPart, nil
}

func executePartedFallback(ctx context.Context, disk, part3Type, part4Type, part3Name, part4Name string) error {
	steps := [][]string{
		{"mklabel", "gpt"},
		{"-a", "optimal", "mkpart", "bios_grub", "1MiB", "2MiB"},
		{"set", "1", "bios_grub", "on"},
		{"-a", "optimal", "mkpart", "ESP", "fat32", "2MiB", "1026MiB"},
		{"set", "2", "esp", "on"},
		{"-a", "optimal", "mkpart", part3Name, "ext4", "1026MiB", "2050MiB"},
		{"-a", "optimal", "mkpart", part4Name, "ext4", "2050MiB", "100%"},
	}

	if part3Type == "fd00" {
		steps = append(steps, []string{"set", "3", "raid", "on"})
	}
	if part4Type == "fd00" {
		steps = append(steps, []string{"set", "4", "raid", "on"})
	} else if part4Type == "8e00" {
		steps = append(steps, []string{"set", "4", "lvm", "on"})
	}

	for _, step := range steps {
		args := append([]string{"-s", disk}, step...)
		if out, err := exec.CommandContext(ctx, "parted", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("parted %v failed: %w (output: %s)", step, err, string(out))
		}
	}
	return nil
}

// buildLVMOnBlockDevice initializes pvcreate, vgcreate, creates logical volumes and formats filesystems.
func buildLVMOnBlockDevice(ctx context.Context, pvDevice string, cfg domain.DeploymentConfig) (*StorageLayoutResult, error) {
	_ = exec.CommandContext(ctx, "wipefs", "-a", "-f", pvDevice).Run()
	_ = exec.CommandContext(ctx, "pvremove", "-y", "-ff", pvDevice).Run()
	pvCmd := exec.CommandContext(ctx, "pvcreate", "-ff", "-y", pvDevice)
	if out, err := pvCmd.CombinedOutput(); err != nil {
		slog.WarnContext(ctx, "pvcreate warning/error", "error", err, "output", string(out))
	}

	vgCmd := exec.CommandContext(ctx, "vgcreate", "-y", "-ff", "vg_system", pvDevice)
	if out, err := vgCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed creating LVM volume group vg_system on %s: %w (output: %s)", pvDevice, err, string(out))
	}

	defaultFSType := "xfs"
	if strings.Contains(strings.ToLower(string(cfg.OS)), "debian") || strings.Contains(strings.ToLower(string(cfg.OS)), "ubuntu") {
		defaultFSType = "ext4"
	}

	vols := cfg.Storage.LVMVolumes
	if len(vols) == 0 {
		vols = []domain.LVMVolumeConfig{
			{Name: "root", MountPoint: "/", SizeGB: 0, FSType: defaultFSType},
		}
	}

	// Query VG free capacity in megabytes for precise boundary calculation
	var freeGB int = 15
	vgsCmd := exec.CommandContext(ctx, "vgs", "--noheadings", "--nosuffix", "--units", "m", "-o", "vg_free", "vg_system")
	if out, err := vgsCmd.Output(); err == nil {
		fields := strings.Fields(string(out))
		if len(fields) > 0 {
			cleaned := strings.Trim(fields[0], "<> \t\r\n")
			cleaned = strings.ReplaceAll(cleaned, ",", ".")
			if freeMB, err := strconv.ParseFloat(cleaned, 64); err == nil && freeMB > 0 {
				freeGB = int(freeMB / 1024)
			}
		}
	}

	swapGB := cfg.Storage.SwapSizeGB
	totalReq := swapGB
	for _, v := range vols {
		totalReq += v.SizeGB
	}

	if totalReq > freeGB && freeGB > 0 {
		return nil, fmt.Errorf("%w: requested total %d GB (volumes %d GB, swap %d GB), but Volume Group vg_system has only %d GB free",
			domain.ErrInsufficientStorage, totalReq, totalReq-swapGB, swapGB, freeGB)
	}

	// Create Swap LV
	var swapDev string
	if swapGB > 0 {
		lvSwapCmd := exec.CommandContext(ctx, "lvcreate", "-y", "-L", fmt.Sprintf("%dG", swapGB), "-n", "swap", "vg_system")
		if out, err := lvSwapCmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("failed creating swap logical volume (%dG): %w (output: %s)", swapGB, err, string(out))
		}
	}

	// Order volumes so fixed-size volumes are created first, followed by the +100%FREE volume
	var fixedVols []domain.LVMVolumeConfig
	var fillVols []domain.LVMVolumeConfig
	for _, v := range vols {
		if v.SizeGB > 0 {
			fixedVols = append(fixedVols, v)
		} else {
			fillVols = append(fillVols, v)
		}
	}
	orderedVols := append(fixedVols, fillVols...)

	// Create Logical Volumes deterministically
	for _, vol := range orderedVols {
		var lvCreate *exec.Cmd
		if vol.SizeGB <= 0 {
			lvCreate = exec.CommandContext(ctx, "lvcreate", "-y", "-l", "+100%FREE", "-n", vol.Name, "vg_system")
		} else {
			lvCreate = exec.CommandContext(ctx, "lvcreate", "-y", "-L", fmt.Sprintf("%dG", vol.SizeGB), "-n", vol.Name, "vg_system")
		}

		if out, err := lvCreate.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("failed creating logical volume %s (%d GB): %w (output: %s)", vol.Name, vol.SizeGB, err, string(out))
		}
	}

	_ = exec.CommandContext(ctx, "vgchange", "-ay", "vg_system").Run()
	_ = exec.CommandContext(ctx, "vgmknodes").Run()
	settlePartitions(ctx, pvDevice)
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
			if strings.Contains(strings.ToLower(string(cfg.OS)), "debian") || strings.Contains(strings.ToLower(string(cfg.OS)), "ubuntu") {
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
	_ = exec.CommandContext(ctx, "udevadm", "settle", "--timeout=10").Run()
}

// waitForDevice polls until a device node exists in /dev or timeout expires.
func waitForDevice(ctx context.Context, devPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(devPath); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	_ = exec.CommandContext(ctx, "udevadm", "settle", "--timeout=2").Run()
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

	var sb strings.Builder
	sb.WriteString("# RedWolf Software RAID Configuration\n")
	sb.WriteString("DEVICE partitions\n")
	sb.Write(out)
	sb.WriteString("\n")
	data := []byte(sb.String())

	for i, confFile := range confFiles {
		if err := os.MkdirAll(confDirs[i], 0755); err != nil {
			continue
		}
		_ = os.WriteFile(confFile, data, 0644)
	}

	return nil
}

// getMDArrayUUID queries the mdadm array details and extracts its MD_UUID value.
func getMDArrayUUID(ctx context.Context, mdDev string) string {
	cmd := exec.CommandContext(ctx, "mdadm", "--detail", "--export", mdDev)
	out, err := cmd.Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "MD_UUID=") {
				val := strings.Trim(strings.TrimPrefix(trimmed, "MD_UUID="), "\"")
				if val != "" {
					return val
				}
			}
		}
	}

	detailCmd := exec.CommandContext(ctx, "mdadm", "--detail", mdDev)
	detailOut, detailErr := detailCmd.Output()
	if detailErr == nil {
		for _, line := range strings.Split(string(detailOut), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "UUID :") {
				val := strings.TrimSpace(strings.TrimPrefix(trimmed, "UUID :"))
				if val != "" {
					return val
				}
			}
		}
	}
	return ""
}

// ensureMDDeviceNode creates a compatibility symlink if udev or devtmpfs created an alternate device node name.
func ensureMDDeviceNode(mdPath string, index int) {
	if _, err := os.Stat(mdPath); os.IsNotExist(err) {
		altPath := fmt.Sprintf("/dev/md/%d", index)
		if _, err2 := os.Stat(altPath); err2 == nil {
			_ = os.Symlink(altPath, mdPath)
			return
		}
		altPath127 := fmt.Sprintf("/dev/md%d", 127-index)
		if _, err3 := os.Stat(altPath127); err3 == nil {
			_ = os.Symlink(altPath127, mdPath)
		}
	}
}
