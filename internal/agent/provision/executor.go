package provision

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// ProgressReporter sends live updates to the RedWolf Core API.
type ProgressReporter interface {
	Report(ctx context.Context, nodeID string, progress int, stage, logMsg string) error
}

// ExecuteDeployment coordinates the entire bare-metal provisioning lifecycle.
func ExecuteDeployment(ctx context.Context, task *domain.DeploymentTask, bootMAC string, reporter ProgressReporter) error {
	nodeID := task.NodeID
	targetDrive := task.TargetDrivePath

	slog.InfoContext(ctx, "starting execution of bare-metal deployment task",
		"task_id", task.TaskID,
		"node_id", nodeID,
		"os", task.OS,
		"drive", targetDrive,
		"preset", task.Config.PartitioningPreset,
		"layout_mode", task.Config.Storage.LayoutMode,
		"raid_level", task.Config.Storage.RAIDLevel,
	)

	// Step 1: Storage Architecture & Topology Preparation (Single disk, mdraid, or LVM)
	_ = reporter.Report(ctx, nodeID, 5, "Preparing block storage target...", "Configuring storage topology and sanitizing block targets...")
	layout, err := SetupStorageArchitecture(ctx, task.Config)
	if err != nil {
		errMsg := fmt.Sprintf("Storage architecture setup failed: %v", err)
		slog.ErrorContext(ctx, "storage architecture setup error", "error", err)
		_ = reporter.Report(ctx, nodeID, 0, "Storage Setup Failed", errMsg)
		return fmt.Errorf("storage setup error: %w", err)
	}

	// Route based on storage architecture (LVM/RAID extraction vs Standard Block Stream)
	if layout.IsLVM || layout.IsSoftwareRAID {
		if err := executeExtractDeployment(ctx, task, layout, bootMAC, reporter); err != nil {
			errMsg := fmt.Sprintf("Deployment failed: %v", err)
			_ = reporter.Report(ctx, nodeID, 0, "Deployment Failed", errMsg)
			return err
		}
	} else {
		if err := executeStandardDeployment(ctx, task, layout, bootMAC, reporter); err != nil {
			errMsg := fmt.Sprintf("Deployment failed: %v", err)
			_ = reporter.Report(ctx, nodeID, 0, "Deployment Failed", errMsg)
			return err
		}
	}

	// Finalize storage subsystem, flush all buffers, and cleanly stop RAID arrays prior to reboot
	CleanShutdownStorage(ctx, "/mnt/redwolf-target", layout)

	// Finalize and trigger reboot into production OS
	_ = reporter.Report(ctx, nodeID, 100, "Active in Production", "Provisioning complete. Rebooting into installed operating system.")
	slog.InfoContext(ctx, "bare-metal provisioning complete; issuing reboot signal", "node_id", nodeID)

	time.Sleep(2 * time.Second)
	rebootCmd := exec.CommandContext(ctx, "reboot", "-f")
	_ = rebootCmd.Run()

	return nil
}

// executeStandardDeployment streams the compressed raw image directly to the target disk (Tier 1 block streaming),
// auto-expands the root partition, and writes cloud-init configurations.
func executeStandardDeployment(ctx context.Context, task *domain.DeploymentTask, layout *StorageLayoutResult, bootMAC string, reporter ProgressReporter) error {
	nodeID := task.NodeID
	streamTarget := layout.TargetDrive
	if streamTarget == "" {
		streamTarget = task.TargetDrivePath
	}

	onProgress := func(pct int, writtenBytes int64, msg string) {
		_ = reporter.Report(ctx, nodeID, pct, fmt.Sprintf("Streaming %s...", task.OS), msg)
	}

	// Pre-wipe target disk to eliminate previous partition tables and headers
	_ = reporter.Report(ctx, nodeID, 5, "Preparing target disk...", fmt.Sprintf("Wiping signatures and partition tables on %s", streamTarget))
	if err := WipeTargetDisk(ctx, streamTarget); err != nil {
		slog.WarnContext(ctx, "non-fatal warning during pre-wipe of target disk", "drive", streamTarget, "error", err)
	}

	// Stream image
	if err := StreamImage(ctx, task.ImageURL, streamTarget, onProgress); err != nil {
		return fmt.Errorf("streaming error: %w", err)
	}

	// Relocate secondary GPT header and expand root partition entry to end of disk
	_ = reporter.Report(ctx, nodeID, 75, "Auto-expanding root partition...", "Relocating GPT header and expanding root partition to 100% capacity")
	if err := RepairGPTHeader(ctx, streamTarget, task.OS); err != nil {
		slog.WarnContext(ctx, "non-fatal warning during gpt repair and expansion", "error", err)
	}

	// Direct Cloud-Init NoCloud injection & online filesystem resize
	_ = reporter.Report(ctx, nodeID, 85, "Injecting Cloud-Init NoCloud seed...", "Writing user-data, meta-data, and network-config")
	if err := InjectCloudInit(ctx, streamTarget, task.Config, bootMAC); err != nil {
		return fmt.Errorf("cloud-init injection error: %w", err)
	}

	// Register UEFI bootloader in NVRAM for all member disks
	_ = reporter.Report(ctx, nodeID, 95, "Registering UEFI boot entry...", "Configuring NVRAM with efibootmgr")
	for _, espTarget := range layout.ESPDrives {
		if err := ConfigureBootloader(ctx, espTarget, task.OS); err != nil {
			slog.WarnContext(ctx, "non-fatal warning during bootloader config", "drive", espTarget, "error", err)
		}
	}

	return nil
}

// executeExtractDeployment extracts and synchronizes the OS distribution image to structured target volumes
// (LVM logical volumes or Software RAID partitions), generates clean /etc/fstab, injects cloud-init, and sets up UEFI.
func executeExtractDeployment(ctx context.Context, task *domain.DeploymentTask, layout *StorageLayoutResult, bootMAC string, reporter ProgressReporter) error {
	nodeID := task.NodeID
	targetRootMount := "/mnt/redwolf-target"
	if err := os.MkdirAll(targetRootMount, 0755); err != nil {
		return fmt.Errorf("failed creating mount directory %s: %w", targetRootMount, err)
	}

	// 1. Mount root filesystem (Logical Volume or root RAID device)
	rootDevice := layout.RootPartition
	if rootDevice == "" {
		if layout.IsLVM {
			rootDevice = resolveLVMDeviceNode("vg_system", "root")
		} else {
			rootDevice = layout.TargetDrive
		}
	}

	for _, mod := range []string{"dm_mod", "md_mod", "xfs", "ext4"} {
		_ = exec.CommandContext(ctx, "modprobe", mod).Run()
	}

	rootFSType := layout.RootFSType
	if rootFSType == "" {
		for _, v := range layout.LVMVolumes {
			if v.MountPoint == "/" || v.Name == "root" {
				rootFSType = v.FSType
				break
			}
		}
	}
	if rootFSType == "" {
		rootFSType = "xfs"
	}

	_ = reporter.Report(ctx, nodeID, 15, "Mounting target filesystem hierarchy...", fmt.Sprintf("Mounting root volume %s (%s) to %s", rootDevice, rootFSType, targetRootMount))
	if out, err := exec.CommandContext(ctx, "mount", "-t", rootFSType, rootDevice, targetRootMount).CombinedOutput(); err != nil {
		if fallbackOut, fallbackErr := exec.CommandContext(ctx, "mount", rootDevice, targetRootMount).CombinedOutput(); fallbackErr != nil {
			return fmt.Errorf("failed mounting root volume %s: %w (output: %s; with -t %s: %s)", rootDevice, fallbackErr, string(fallbackOut), rootFSType, string(out))
		}
	}

	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		unmountAllUnder(cleanupCtx, targetRootMount)
	}()

	// 2. Mount /boot and /boot/efi
	bootMount := filepath.Join(targetRootMount, "boot")
	_ = os.MkdirAll(bootMount, 0755)
	bootFSType := layout.BootFSType
	if bootFSType == "" {
		if strings.Contains(strings.ToLower(string(task.OS)), "debian") {
			bootFSType = "ext4"
		} else {
			bootFSType = "xfs"
		}
	}
	if layout.BootPartition != "" {
		if out, err := exec.CommandContext(ctx, "mount", "-t", bootFSType, layout.BootPartition, bootMount).CombinedOutput(); err != nil {
			if out2, err2 := exec.CommandContext(ctx, "mount", layout.BootPartition, bootMount).CombinedOutput(); err2 != nil {
				slog.WarnContext(ctx, "failed mounting boot partition", "partition", layout.BootPartition, "output", string(out), "fallback_output", string(out2))
			}
		}
	}

	efiMount := filepath.Join(bootMount, "efi")
	_ = os.MkdirAll(efiMount, 0755)
	if layout.ESPPartition != "" {
		if out, err := exec.CommandContext(ctx, "mount", "-t", "vfat", layout.ESPPartition, efiMount).CombinedOutput(); err != nil {
			if out2, err2 := exec.CommandContext(ctx, "mount", layout.ESPPartition, efiMount).CombinedOutput(); err2 != nil {
				slog.WarnContext(ctx, "failed mounting efi partition", "partition", layout.ESPPartition, "output", string(out), "fallback_output", string(out2))
			}
		}
	}

	// 3. Mount additional Logical Volumes (e.g. /home, /var)
	var subMounts []lvmMountEntry
	for _, vol := range layout.LVMVolumes {
		mp := strings.TrimSpace(vol.MountPoint)
		if mp == "" || mp == "/" {
			continue
		}
		targetPath := filepath.Join(targetRootMount, mp)
		_ = os.MkdirAll(targetPath, 0755)
		lvDev := resolveLVMDeviceNode("vg_system", vol.Name)
		fsType := vol.FSType
		if fsType == "" {
			fsType = "xfs"
		}
		if out, err := exec.CommandContext(ctx, "mount", "-t", fsType, lvDev, targetPath).CombinedOutput(); err != nil {
			if out2, err2 := exec.CommandContext(ctx, "mount", lvDev, targetPath).CombinedOutput(); err2 != nil {
				slog.WarnContext(ctx, "failed mounting sub volume", "lv", lvDev, "mount", targetPath, "output", string(out), "fallback_output", string(out2))
			}
		}
		subMounts = append(subMounts, lvmMountEntry{
			device:     lvDev,
			mountPoint: mp,
			fsType:     fsType,
		})
	}

	// 4. Stream and unpack OS image to loop container
	_ = reporter.Report(ctx, nodeID, 25, "Streaming distribution image...", "Streaming and decompressing raw OS image to loop container")
	tmpImage := selectTempImagePath(targetRootMount)
	onProgress := func(pct int, writtenBytes int64, msg string) {
		scaled := 25 + int(float64(pct)*0.4) // Scaled between 25% and 65%
		_ = reporter.Report(ctx, nodeID, scaled, fmt.Sprintf("Streaming %s...", task.OS), msg)
	}

	if err := StreamImage(ctx, task.ImageURL, tmpImage, onProgress); err != nil {
		_ = os.Remove(tmpImage)
		return fmt.Errorf("failed streaming image: %w", err)
	}
	defer os.Remove(tmpImage)

	// 5. Attach loop device
	_ = reporter.Report(ctx, nodeID, 65, "Extracting distribution rootfs...", "Mounting source image loop container")
	loopCmd := exec.CommandContext(ctx, "losetup", "-P", "-r", "-f", "--show", tmpImage)
	loopOut, err := loopCmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(tmpImage)
		return fmt.Errorf("losetup failed on %s: %w (output: %s)", tmpImage, err, string(loopOut))
	}
	loopDev := strings.TrimSpace(string(loopOut))
	defer func() {
		_ = exec.CommandContext(context.Background(), "losetup", "-d", loopDev).Run()
	}()

	settlePartitions(ctx, loopDev)

	// 6. Copy files from source partitions into target filesystem hierarchy
	sourceRootPart, err := extractImageToTarget(ctx, loopDev, targetRootMount, reporter, nodeID, task.OS)
	if err != nil {
		return fmt.Errorf("failed extracting source image to target: %w", err)
	}

	// Capture source root partition UUID before detaching loop device
	sourceRootUUID := getPartitionUUID(ctx, sourceRootPart)

	// Explicitly detach loop device and delete temporary image to free memory/disk immediately
	_ = exec.CommandContext(ctx, "losetup", "-d", loopDev).Run()
	_ = os.Remove(tmpImage)

	// 7. Update Bootloader root parameters (BLS entries / grub.cfg)
	updateBootloaderConfigs(ctx, targetRootMount, layout, sourceRootUUID)

	// 8. Generate clean /etc/fstab
	_ = reporter.Report(ctx, nodeID, 80, "Generating /etc/fstab...", "Writing filesystem table with device mapper and partition UUIDs")
	if err := generateLVMFstab(ctx, targetRootMount, layout, subMounts); err != nil {
		slog.WarnContext(ctx, "warning generating fstab", "error", err)
	}

	// 9. Inject mdadm.conf if Software RAID is configured
	if layout.IsSoftwareRAID {
		_ = reporter.Report(ctx, nodeID, 83, "Synchronizing RAID configuration...", "Writing mdadm.conf to rootfs")
		if err := InjectMDADMConfig(ctx, targetRootMount); err != nil {
			slog.WarnContext(ctx, "warning injecting mdadm.conf into rootfs", "error", err)
		}
	}

	// 10. Configure LVM device scanning and purge stale cloud-image devices files
	if layout.IsLVM {
		_ = ConfigureTargetLVM(targetRootMount)
	}

	// 11. Inject Cloud-Init NoCloud seed & direct credentials + /etc/redwolf-release
	_ = reporter.Report(ctx, nodeID, 85, "Injecting Cloud-Init NoCloud seed...", "Writing user-data, meta-data, and network-config")
	if err := WriteNoCloudSeeds(targetRootMount, task.Config, bootMAC); err != nil {
		return fmt.Errorf("failed injecting cloud-init: %w", err)
	}
	if err := DirectInjectSecurityCredentials(targetRootMount, task.Config); err != nil {
		slog.WarnContext(ctx, "non-fatal warning injecting security credentials", "error", err)
	}

	// 12. Regenerate target initramfs with baked-in RAID and LVM modules
	if layout.IsSoftwareRAID || layout.IsLVM {
		_ = reporter.Report(ctx, nodeID, 87, "Regenerating initramfs...", "Building initramfs with storage and RAID drivers")
		tryRebuildInitramfs(ctx, targetRootMount)
	}

	// 13. Install Legacy BIOS MBR bootloader on all target drives (supports dual BIOS & UEFI booting)
	_ = reporter.Report(ctx, nodeID, 88, "Installing BIOS MBR bootloader...", "Writing MBR boot code and core image to target drives")
	if err := InstallBIOSBootloader(ctx, layout.ESPDrives, bootMount, targetRootMount); err != nil {
		slog.WarnContext(ctx, "non-fatal warning during BIOS bootloader installation", "error", err)
	}

	// 14. Generate Universal GRUB configuration (guarantees direct kernel boot on BIOS & UEFI)
	_ = reporter.Report(ctx, nodeID, 89, "Generating universal bootloader configuration...", "Writing direct kernel boot entries to GRUB and EFI")
	if err := GenerateUniversalGrubConfig(ctx, targetRootMount, layout, task.OS); err != nil {
		slog.WarnContext(ctx, "warning generating universal grub config", "error", err)
	}

	// 15. Unmount all target partitions cleanly
	_ = reporter.Report(ctx, nodeID, 90, "Finalizing storage writes...", "Flushing disk buffers and unmounting target volumes")
	unmountAllUnder(ctx, targetRootMount)

	// 12. For multi-disk Software RAID, synchronize UEFI boot files to all member ESPs
	if layout.IsSoftwareRAID && len(layout.MemberESPs) > 1 {
		_ = reporter.Report(ctx, nodeID, 92, "Synchronizing RAID member ESPs...", "Writing redundant UEFI bootloaders to all drive ESPs")
		if err := syncMemberESPs(ctx, layout.MemberESPs); err != nil {
			slog.WarnContext(ctx, "non-fatal warning synchronizing member ESPs", "error", err)
		}
	}

	// 13. Register UEFI boot entry in NVRAM for all member disks
	_ = reporter.Report(ctx, nodeID, 95, "Registering UEFI boot entry...", "Configuring NVRAM with efibootmgr")
	for _, espTarget := range layout.ESPDrives {
		if err := ConfigureBootloader(ctx, espTarget, task.OS); err != nil {
			slog.WarnContext(ctx, "non-fatal warning during bootloader config", "drive", espTarget, "error", err)
		}
	}

	return nil
}

// executeLVMDeployment is an alias retained for package compatibility.
func executeLVMDeployment(ctx context.Context, task *domain.DeploymentTask, layout *StorageLayoutResult, bootMAC string, reporter ProgressReporter) error {
	return executeExtractDeployment(ctx, task, layout, bootMAC, reporter)
}

func syncMemberESPs(ctx context.Context, memberESPs []string) error {
	if len(memberESPs) < 2 {
		return nil
	}
	primaryESP := memberESPs[0]
	primaryMount := "/mnt/redwolf-primary-esp"
	_ = os.MkdirAll(primaryMount, 0755)

	if out, err := exec.CommandContext(ctx, "mount", "-t", "vfat", "-o", "ro", primaryESP, primaryMount).CombinedOutput(); err != nil {
		return fmt.Errorf("failed mounting primary ESP %s: %w (output: %s)", primaryESP, err, string(out))
	}
	defer func() {
		_ = exec.CommandContext(context.Background(), "umount", primaryMount).Run()
	}()

	efiSrc := filepath.Join(primaryMount, "EFI")
	if fi, err := os.Stat(efiSrc); err != nil || !fi.IsDir() {
		return fmt.Errorf("EFI directory not found in primary ESP %s", primaryESP)
	}

	targetMount := "/mnt/redwolf-member-esp"
	_ = os.MkdirAll(targetMount, 0755)

	for _, espPart := range memberESPs[1:] {
		_ = exec.CommandContext(ctx, "mkfs.vfat", "-F32", espPart).Run()
		if out, err := exec.CommandContext(ctx, "mount", "-t", "vfat", espPart, targetMount).CombinedOutput(); err == nil {
			targetEFI := filepath.Join(targetMount, "EFI")
			_ = os.MkdirAll(targetEFI, 0755)
			_ = exec.CommandContext(ctx, "cp", "-a", efiSrc+"/.", targetEFI+"/").Run()
			_ = exec.CommandContext(ctx, "umount", targetMount).Run()
			slog.InfoContext(ctx, "synchronized redundant UEFI bootloader to RAID member ESP", "partition", espPart)
		} else {
			slog.WarnContext(ctx, "failed mounting member ESP", "partition", espPart, "output", string(out))
		}
	}
	return nil
}

func extractImageToTarget(ctx context.Context, loopDev, targetRootMount string, reporter ProgressReporter, nodeID string, osType domain.OperatingSystem) (string, error) {
	// Source root detection
	sourceRootPart, err := findRootPartition(ctx, loopDev, osType)
	if err != nil {
		sourceRootPart = fallbackPartitionPath(loopDev, osType)
	}
	_ = waitForDevice(ctx, sourceRootPart, 5*time.Second)

	srcRootMount := "/mnt/redwolf-source-root"
	_ = os.MkdirAll(srcRootMount, 0755)
	if err := MountTargetFilesystem(ctx, sourceRootPart, srcRootMount, osType, "-o", "ro"); err != nil {
		return "", fmt.Errorf("failed mounting source root %s: %w", sourceRootPart, err)
	}
	defer func() {
		_ = exec.CommandContext(context.Background(), "umount", srcRootMount).Run()
	}()

	_ = reporter.Report(ctx, nodeID, 70, "Synchronizing root filesystem...", "Copying operating system root files into target volumes")
	cpRootCmd := exec.CommandContext(ctx, "cp", "-a", srcRootMount+"/.", targetRootMount+"/")
	if out, err := cpRootCmd.CombinedOutput(); err != nil {
		slog.WarnContext(ctx, "cp rootfs warning", "error", err, "output", string(out))
	}

	// Source boot partition (partition 3 on AlmaLinux)
	srcBootPart := resolvePartitionPath(loopDev, 3)
	_ = waitForDevice(ctx, srcBootPart, 2*time.Second)
	if _, err := os.Stat(srcBootPart); err == nil && !strings.Contains(strings.ToLower(string(osType)), "debian") {
		srcBootMount := "/mnt/redwolf-source-boot"
		_ = os.MkdirAll(srcBootMount, 0755)
		if out, err := exec.CommandContext(ctx, "mount", "-o", "ro", srcBootPart, srcBootMount).CombinedOutput(); err == nil {
			_ = reporter.Report(ctx, nodeID, 75, "Synchronizing /boot partition...", "Copying kernel and initramfs to /boot")
			_ = exec.CommandContext(ctx, "cp", "-a", srcBootMount+"/.", filepath.Join(targetRootMount, "boot")+"/").Run()
			_ = exec.CommandContext(ctx, "umount", srcBootMount).Run()
		} else {
			slog.DebugContext(ctx, "source boot mount skipped", "output", string(out))
		}
	}

	// Source EFI partition (partition 2 on AlmaLinux, 15 on Debian)
	efiPartNum := detectEFIPartition(ctx, loopDev, osType)
	srcEFIPart := resolvePartitionPath(loopDev, efiPartNum)
	_ = waitForDevice(ctx, srcEFIPart, 2*time.Second)
	if _, err := os.Stat(srcEFIPart); err == nil {
		srcEFIMount := "/mnt/redwolf-source-efi"
		_ = os.MkdirAll(srcEFIMount, 0755)
		if out, err := exec.CommandContext(ctx, "mount", "-o", "ro", srcEFIPart, srcEFIMount).CombinedOutput(); err == nil {
			_ = reporter.Report(ctx, nodeID, 78, "Synchronizing EFI system files...", "Copying UEFI shim and GRUB to /boot/efi")
			_ = exec.CommandContext(ctx, "cp", "-a", srcEFIMount+"/.", filepath.Join(targetRootMount, "boot", "efi")+"/").Run()
			_ = exec.CommandContext(ctx, "umount", srcEFIMount).Run()
		} else {
			slog.DebugContext(ctx, "source efi mount skipped", "output", string(out))
		}
	}

	return sourceRootPart, nil
}

type lvmMountEntry struct {
	device     string
	mountPoint string
	fsType     string
}

func canonicalMapperDev(dev string) string {
	if strings.HasPrefix(dev, "/dev/mapper/") {
		return dev
	}
	if strings.HasPrefix(dev, "/dev/") {
		parts := strings.Split(strings.TrimPrefix(dev, "/dev/"), "/")
		if len(parts) == 2 {
			return fmt.Sprintf("/dev/mapper/%s-%s",
				strings.ReplaceAll(parts[0], "-", "--"),
				strings.ReplaceAll(parts[1], "-", "--"),
			)
		}
	}
	return dev
}

func generateLVMFstab(ctx context.Context, targetRootMount string, layout *StorageLayoutResult, subMounts []lvmMountEntry) error {
	fstabPath := filepath.Join(targetRootMount, "etc", "fstab")
	_ = os.MkdirAll(filepath.Dir(fstabPath), 0755)

	bootUUID := getPartitionUUID(ctx, layout.BootPartition)
	espUUID := getPartitionUUID(ctx, layout.ESPPartition)

	rootFs := layout.RootFSType
	if rootFs == "" {
		rootFs = "xfs"
	}

	bootFs := layout.BootFSType
	if bootFs == "" {
		bootFs = "xfs"
	}

	var sb strings.Builder
	sb.WriteString("# /etc/fstab: auto-generated by RedWolf Bare-Metal Provisioning Engine\n")
	sb.WriteString("# <file system> <mount point> <type> <options> <dump> <pass>\n")

	isLVM := layout.IsLVM || len(layout.LVMVolumes) > 0 || layout.LVMVolumeGroup != ""
	if isLVM {
		rootDev := layout.RootPartition
		if rootDev == "" {
			rootDev = "/dev/mapper/vg_system-root"
		} else {
			rootDev = canonicalMapperDev(rootDev)
		}
		sb.WriteString(fmt.Sprintf("%s / %s defaults 0 0\n", rootDev, rootFs))

		for _, sm := range subMounts {
			fs := sm.fsType
			if fs == "" {
				fs = "xfs"
			}
			devNode := canonicalMapperDev(sm.device)
			sb.WriteString(fmt.Sprintf("%s %s %s defaults 0 0\n", devNode, sm.mountPoint, fs))
		}

		if layout.SwapDevice != "" {
			swapDev := canonicalMapperDev(layout.SwapDevice)
			sb.WriteString(fmt.Sprintf("%s none swap sw 0 0\n", swapDev))
		}
	} else if layout.IsSoftwareRAID {
		rootUUID := getPartitionUUID(ctx, layout.RootPartition)
		if rootUUID != "" {
			sb.WriteString(fmt.Sprintf("UUID=%s / %s defaults 0 0\n", rootUUID, rootFs))
		} else {
			sb.WriteString(fmt.Sprintf("%s / %s defaults 0 0\n", layout.RootPartition, rootFs))
		}
	}

	if bootUUID != "" {
		sb.WriteString(fmt.Sprintf("UUID=%s /boot %s defaults 0 0\n", bootUUID, bootFs))
	} else if layout.BootPartition != "" {
		sb.WriteString(fmt.Sprintf("%s /boot %s defaults 0 0\n", layout.BootPartition, bootFs))
	}

	if espUUID != "" {
		sb.WriteString(fmt.Sprintf("UUID=%s /boot/efi vfat umask=0077,shortname=winnt 0 2\n", espUUID))
	} else if layout.ESPPartition != "" {
		sb.WriteString(fmt.Sprintf("%s /boot/efi vfat umask=0077,shortname=winnt 0 2\n", layout.ESPPartition))
	}

	return os.WriteFile(fstabPath, []byte(sb.String()), 0644)
}

func updateBootloaderConfigs(ctx context.Context, targetRootMount string, layout *StorageLayoutResult, sourceRootUUID string) {
	newRootUUID := getPartitionUUID(ctx, layout.RootPartition)
	kernelArgs := ComputeKernelArgs(layout, newRootUUID)

	// 1. Update BLS entries (/boot/loader/entries/*.conf)
	entriesDir := filepath.Join(targetRootMount, "boot", "loader", "entries")
	if entries, err := os.ReadDir(entriesDir); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".conf") {
				p := filepath.Join(entriesDir, e.Name())
				if data, err := os.ReadFile(p); err == nil {
					lines := strings.Split(string(data), "\n")
					for idx, line := range lines {
						trimmed := strings.TrimSpace(line)
						if strings.HasPrefix(trimmed, "options ") {
							lines[idx] = "options " + kernelArgs
						}
					}
					_ = os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0644)
				}
			}
		}
	}

	// 2. Update Debian / GRUB configs (/boot/grub/grub.cfg, /boot/grub2/grub.cfg)
	grubPaths := []string{
		filepath.Join(targetRootMount, "boot", "grub", "grub.cfg"),
		filepath.Join(targetRootMount, "boot", "grub2", "grub.cfg"),
	}
	for _, gp := range grubPaths {
		if data, err := os.ReadFile(gp); err == nil && sourceRootUUID != "" {
			content := string(data)
			if newRootUUID != "" {
				content = strings.ReplaceAll(content, sourceRootUUID, newRootUUID)
			}
			_ = os.WriteFile(gp, []byte(content), 0644)
		}
	}

	// 3. Update all EFI stub grub.cfg files on the mounted EFI System Partition (/boot/efi/EFI)
	newBootUUID := getPartitionUUID(ctx, layout.BootPartition)
	if newBootUUID == "" {
		newBootUUID = newRootUUID
	}

	efiBase := filepath.Join(targetRootMount, "boot", "efi", "EFI")
	if efiEntries, err := os.ReadDir(efiBase); err == nil {
		var shimPath, grubBinPath string

		for _, entry := range efiEntries {
			if !entry.IsDir() {
				continue
			}
			subDir := filepath.Join(efiBase, entry.Name())
			subCfg := filepath.Join(subDir, "grub.cfg")

			if sPath := filepath.Join(subDir, "shimx64.efi"); shimPath == "" {
				if _, err := os.Stat(sPath); err == nil {
					shimPath = sPath
				}
			}
			if gPath := filepath.Join(subDir, "grubx64.efi"); grubBinPath == "" {
				if _, err := os.Stat(gPath); err == nil {
					grubBinPath = gPath
				}
			}

			if newBootUUID != "" {
				var efiSb strings.Builder
				efiSb.WriteString("insmod part_gpt\ninsmod ext2\ninsmod xfs\ninsmod mdraid1x\ninsmod lvm\n")
				efiSb.WriteString(fmt.Sprintf("search --no-floppy --fs-uuid --set=dev %s\n", newBootUUID))
				efiSb.WriteString("configfile ($dev)/grub2/grub.cfg\n")
				efiSb.WriteString("configfile ($dev)/grub/grub.cfg\n")
				efiSb.WriteString("configfile ($dev)/boot/grub2/grub.cfg\n")
				efiSb.WriteString("configfile ($dev)/boot/grub/grub.cfg\n")
				efiSb.WriteString("configfile ($dev)/grub.cfg\n")
				_ = os.WriteFile(subCfg, []byte(efiSb.String()), 0644)
			}
		}

		// Ensure fallback /boot/efi/EFI/BOOT exists with working BOOTX64.EFI and grub.cfg
		bootDir := filepath.Join(efiBase, "BOOT")
		_ = os.MkdirAll(bootDir, 0755)

		fallbackLoader := filepath.Join(bootDir, "BOOTX64.EFI")
		if _, err := os.Stat(fallbackLoader); os.IsNotExist(err) {
			if shimPath != "" {
				_ = exec.CommandContext(ctx, "cp", "-a", shimPath, fallbackLoader).Run()
			} else if grubBinPath != "" {
				_ = exec.CommandContext(ctx, "cp", "-a", grubBinPath, fallbackLoader).Run()
			}
		}

		fallbackGrubBin := filepath.Join(bootDir, "grubx64.efi")
		if _, err := os.Stat(fallbackGrubBin); os.IsNotExist(err) && grubBinPath != "" {
			_ = exec.CommandContext(ctx, "cp", "-a", grubBinPath, fallbackGrubBin).Run()
		}

		fallbackCfg := filepath.Join(bootDir, "grub.cfg")
		if newBootUUID != "" {
			var fallbackSb strings.Builder
			fallbackSb.WriteString("insmod part_gpt\ninsmod ext2\ninsmod xfs\ninsmod mdraid1x\ninsmod lvm\n")
			fallbackSb.WriteString(fmt.Sprintf("search --no-floppy --fs-uuid --set=dev %s\n", newBootUUID))
			fallbackSb.WriteString("configfile ($dev)/grub2/grub.cfg\n")
			fallbackSb.WriteString("configfile ($dev)/grub/grub.cfg\n")
			fallbackSb.WriteString("configfile ($dev)/boot/grub2/grub.cfg\n")
			fallbackSb.WriteString("configfile ($dev)/boot/grub/grub.cfg\n")
			fallbackSb.WriteString("configfile ($dev)/grub.cfg\n")
			_ = os.WriteFile(fallbackCfg, []byte(fallbackSb.String()), 0644)
		}
	}
}

func getPartitionUUID(ctx context.Context, partDev string) string {
	if partDev == "" {
		return ""
	}
	cmd := exec.CommandContext(ctx, "blkid", "-s", "UUID", "-o", "value", partDev)
	out, err := cmd.Output()
	if err == nil && len(out) > 0 {
		return strings.TrimSpace(string(out))
	}
	return ""
}

func unmountAllUnder(ctx context.Context, rootMount string) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		_ = exec.CommandContext(ctx, "umount", "-R", rootMount).Run()
		return
	}
	lines := strings.Split(string(data), "\n")
	var mounts []string
	for _, l := range lines {
		fields := strings.Fields(l)
		if len(fields) >= 2 {
			mp := fields[1]
			if strings.HasPrefix(mp, rootMount) {
				mounts = append(mounts, mp)
			}
		}
	}
	// Sort by descending string length so deepest nested mount points are unmounted first
	sort.Slice(mounts, func(i, j int) bool {
		return len(mounts[i]) > len(mounts[j])
	})

	for _, m := range mounts {
		_ = exec.CommandContext(ctx, "umount", m).Run()
	}
	_ = exec.CommandContext(ctx, "umount", rootMount).Run()
}

// selectTempImagePath determines the safest location to store the unpacked OS image.
// It prioritizes in-memory RAM (/tmp) if sufficient capacity exists (>= 3.5 GiB free)
// to prevent disk wear, and defensively falls back to the target drive if RAM is constrained.
func selectTempImagePath(targetRootMount string) string {
	var tmpStat syscall.Statfs_t
	var tmpFree int64
	if err := syscall.Statfs("/tmp", &tmpStat); err == nil {
		tmpFree = int64(tmpStat.Bavail) * int64(tmpStat.Bsize)
	}

	var targetStat syscall.Statfs_t
	var targetFree int64
	if err := syscall.Statfs(targetRootMount, &targetStat); err == nil {
		targetFree = int64(targetStat.Bavail) * int64(targetStat.Bsize)
	}

	// Prefer in-memory /tmp if there is at least 3.5 GiB free for rapid in-RAM processing
	if tmpFree >= 3500*1024*1024 {
		return "/tmp/redwolf-cloud-image.raw"
	}

	// If target filesystem has more free space than /tmp and at least 2 GiB free, store on disk
	if targetFree > tmpFree && targetFree >= 2000*1024*1024 {
		return filepath.Join(targetRootMount, ".redwolf-cloud-image.raw")
	}

	return "/tmp/redwolf-cloud-image.raw"
}

// CleanShutdownStorage ensures all filesystem buffers are flushed, LVM volume groups are deactivated,
// and Software RAID arrays are cleanly synchronized and stopped or marked read-only before system reboot.
func CleanShutdownStorage(ctx context.Context, targetRootMount string, layout *StorageLayoutResult) {
	slog.InfoContext(ctx, "performing clean storage subsystem synchronization and shutdown")

	// 1. Unmount all mounted filesystems under target and /mnt
	unmountAllUnder(ctx, targetRootMount)
	unmountAllUnder(ctx, "/mnt")

	// 2. Flush dirty filesystem buffers to storage
	_ = exec.CommandContext(ctx, "sync").Run()

	// 3. Deactivate LVM volume groups so underlying block devices are released
	if layout != nil && (layout.IsLVM || len(layout.LVMVolumes) > 0 || layout.LVMVolumeGroup != "") {
		slog.InfoContext(ctx, "deactivating LVM volume groups prior to reboot")
		vgName := layout.LVMVolumeGroup
		if vgName == "" {
			vgName = "vg_system"
		}
		_ = exec.CommandContext(ctx, "vgchange", "-an", vgName).Run()
		_ = exec.CommandContext(ctx, "vgchange", "-an").Run()
		_ = exec.CommandContext(ctx, "sync").Run()
	}

	// 4. Cleanly stop or protect Software RAID arrays
	if layout != nil && layout.IsSoftwareRAID {
		slog.InfoContext(ctx, "synchronizing and stopping Software RAID arrays prior to reboot")

		// Wait up to 30s for the small /boot RAID1 array (/dev/md0) to complete initial mirror sync
		waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		_ = exec.CommandContext(waitCtx, "mdadm", "--wait", "/dev/md0").Run()
		cancel()

		// Wait for arrays to become clean
		_ = exec.CommandContext(ctx, "mdadm", "--wait-clean", "/dev/md0").Run()
		_ = exec.CommandContext(ctx, "mdadm", "--wait-clean", "/dev/md1").Run()

		// Attempt clean stop of all md arrays to terminate background resync threads
		stopOut0, stopErr0 := exec.CommandContext(ctx, "mdadm", "--stop", "/dev/md0").CombinedOutput()
		if stopErr0 != nil {
			slog.DebugContext(ctx, "mdadm stop /dev/md0 returned error; switching to readonly mode", "output", string(stopOut0), "error", stopErr0)
			_ = exec.CommandContext(ctx, "mdadm", "--readonly", "/dev/md0").Run()
		} else {
			slog.InfoContext(ctx, "cleanly stopped /dev/md0 boot RAID array")
		}

		stopOut1, stopErr1 := exec.CommandContext(ctx, "mdadm", "--stop", "/dev/md1").CombinedOutput()
		if stopErr1 != nil {
			slog.DebugContext(ctx, "mdadm stop /dev/md1 returned error; switching to readonly mode", "output", string(stopOut1), "error", stopErr1)
			_ = exec.CommandContext(ctx, "mdadm", "--readonly", "/dev/md1").Run()
		} else {
			slog.InfoContext(ctx, "cleanly stopped /dev/md1 data RAID array")
		}

		_ = exec.CommandContext(ctx, "mdadm", "--stop", "--scan").Run()
	}

	// 5. Flush hardware write caches on all target disks
	if layout != nil {
		for _, drive := range layout.ESPDrives {
			realDisk, err := filepath.EvalSymlinks(drive)
			if err != nil {
				realDisk = drive
			}
			_ = exec.CommandContext(ctx, "blockdev", "--flushbufs", realDisk).Run()
		}
	}

	// 6. Final sync and brief stabilization pause
	_ = exec.CommandContext(ctx, "sync").Run()
	time.Sleep(1 * time.Second)
	slog.InfoContext(ctx, "storage subsystem cleanly synchronized and ready for restart")
}

// ConfigureTargetLVM ensures LVM on the deployed operating system dynamically detects
// Software RAID and NVMe block devices by purging stale devices files and setting use_devicesfile = 0.
func ConfigureTargetLVM(targetRootMount string) error {
	// 1. Remove stale system.devices inherited from cloud images
	devicesDir := filepath.Join(targetRootMount, "etc", "lvm", "devices")
	_ = os.Remove(filepath.Join(devicesDir, "system.devices"))

	if entries, err := os.ReadDir(devicesDir); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".devices") {
				_ = os.Remove(filepath.Join(devicesDir, e.Name()))
			}
		}
	}

	// 2. Disable use_devicesfile in lvmlocal.conf
	lvmDir := filepath.Join(targetRootMount, "etc", "lvm")
	_ = os.MkdirAll(lvmDir, 0755)

	lvmLocalPath := filepath.Join(lvmDir, "lvmlocal.conf")
	localContent := "# Auto-generated by RedWolf Provisioning Engine\n# Disable devices file to permit dynamic detection of Software RAID and NVMe volumes\ndevices {\n    use_devicesfile = 0\n}\n"
	_ = os.WriteFile(lvmLocalPath, []byte(localContent), 0644)

	// 3. Disable use_devicesfile in lvm.conf if present
	lvmConfPath := filepath.Join(lvmDir, "lvm.conf")
	if data, err := os.ReadFile(lvmConfPath); err == nil {
		content := string(data)
		if strings.Contains(content, "use_devicesfile = 1") {
			content = strings.ReplaceAll(content, "use_devicesfile = 1", "use_devicesfile = 0")
			_ = os.WriteFile(lvmConfPath, []byte(content), 0644)
		}
	}

	return nil
}

// tryRebuildInitramfs attempts to regenerate the initramfs inside target chroot to ensure
// mdraid, lvm, and filesystem drivers are natively baked into the boot image.
func tryRebuildInitramfs(ctx context.Context, targetRootMount string) {
	bootDir := filepath.Join(targetRootMount, "boot")
	kInfo, err := FindInstalledKernel(bootDir)
	if err != nil {
		slog.DebugContext(ctx, "skipping initramfs rebuild; kernel not found", "error", err)
		return
	}

	slog.InfoContext(ctx, "regenerating initramfs inside target chroot", "kernel_version", kInfo.KernelVersion)

	binds := []string{"/dev", "/proc", "/sys"}
	for _, b := range binds {
		targetB := filepath.Join(targetRootMount, b)
		_ = os.MkdirAll(targetB, 0755)
		_ = exec.CommandContext(ctx, "mount", "--bind", b, targetB).Run()
	}
	defer func() {
		for i := len(binds) - 1; i >= 0; i-- {
			_ = exec.CommandContext(context.Background(), "umount", filepath.Join(targetRootMount, binds[i])).Run()
		}
	}()

	// Method 1: Dracut (AlmaLinux / RHEL / CentOS / Fedora)
	dracutPath := filepath.Join(targetRootMount, "usr", "bin", "dracut")
	if _, err := os.Stat(dracutPath); err == nil {
		cmd := exec.CommandContext(ctx, "chroot", targetRootMount, "dracut", "--force", "--no-hostonly", "--kver", kInfo.KernelVersion, "--add", "mdraid lvm")
		out, err := cmd.CombinedOutput()
		if err == nil {
			slog.InfoContext(ctx, "successfully regenerated dracut initramfs inside chroot", "kernel_version", kInfo.KernelVersion)
			return
		}
		slog.DebugContext(ctx, "dracut with --no-hostonly warning; retrying standard dracut", "output", string(out), "error", err)
		cmd = exec.CommandContext(ctx, "chroot", targetRootMount, "dracut", "--force", "--kver", kInfo.KernelVersion, "--add", "mdraid lvm")
		out, err = cmd.CombinedOutput()
		if err == nil {
			slog.InfoContext(ctx, "successfully regenerated dracut initramfs inside chroot", "kernel_version", kInfo.KernelVersion)
			return
		}
		slog.DebugContext(ctx, "dracut regeneration in chroot returned warning", "output", string(out), "error", err)
	}

	// Method 2: update-initramfs (Debian / Ubuntu)
	updateInitramfsPath := filepath.Join(targetRootMount, "usr", "sbin", "update-initramfs")
	if _, err := os.Stat(updateInitramfsPath); err == nil {
		cmd := exec.CommandContext(ctx, "chroot", targetRootMount, "update-initramfs", "-u", "-k", kInfo.KernelVersion)
		out, err := cmd.CombinedOutput()
		if err == nil {
			slog.InfoContext(ctx, "successfully updated debian initramfs inside chroot", "kernel_version", kInfo.KernelVersion)
			return
		}
		slog.DebugContext(ctx, "update-initramfs in chroot returned warning", "output", string(out), "error", err)
	}
}
