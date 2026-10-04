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
	)

	// Step 1: Storage Architecture & Topology Preparation (Single disk, mdraid, or LVM)
	_ = reporter.Report(ctx, nodeID, 5, "Preparing block storage target...", "Configuring storage topology and sanitizing block targets...")
	layout, err := SetupStorageArchitecture(ctx, task.Config)
	if err != nil {
		slog.WarnContext(ctx, "storage architecture setup warning; proceeding with fallback", "error", err)
		layout = &StorageLayoutResult{
			TargetDrive: targetDrive,
			ESPDrives:   []string{targetDrive},
			TargetDisk:  targetDrive,
		}
	}

	// Route based on storage architecture (LVM vs Standard Block Stream)
	if layout.IsLVM {
		if err := executeLVMDeployment(ctx, task, layout, bootMAC, reporter); err != nil {
			errMsg := fmt.Sprintf("LVM deployment failed: %v", err)
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

	// Finalize and trigger reboot into production OS
	_ = reporter.Report(ctx, nodeID, 100, "Active in Production", "Provisioning complete. Rebooting into installed operating system.")
	slog.InfoContext(ctx, "bare-metal provisioning complete; issuing reboot signal", "node_id", nodeID)

	time.Sleep(2 * time.Second)
	rebootCmd := exec.CommandContext(ctx, "reboot", "-f")
	_ = rebootCmd.Run()

	return nil
}

// executeStandardDeployment streams the compressed raw image directly to the target disk,
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

	// Stream image
	if err := StreamImage(ctx, task.ImageURL, streamTarget, onProgress); err != nil {
		return fmt.Errorf("streaming error: %w", err)
	}

	// Relocate secondary GPT header and expand root partition entry to end of disk
	_ = reporter.Report(ctx, nodeID, 75, "Auto-expanding root partition...", "Relocating GPT header and expanding root partition to 100% capacity")
	if err := RepairGPTHeader(ctx, streamTarget); err != nil {
		slog.WarnContext(ctx, "non-fatal warning during gpt repair and expansion", "error", err)
	}

	// Direct Cloud-Init NoCloud injection & online filesystem resize
	_ = reporter.Report(ctx, nodeID, 85, "Injecting Cloud-Init NoCloud seed...", "Writing user-data, meta-data, and network-config")
	if err := InjectCloudInit(ctx, streamTarget, task.Config, bootMAC); err != nil {
		return fmt.Errorf("cloud-init injection error: %w", err)
	}

	// For multi-disk Software RAID, synchronize UEFI boot files to all member ESPs
	if layout.IsSoftwareRAID && len(layout.MemberESPs) > 0 {
		_ = reporter.Report(ctx, nodeID, 90, "Synchronizing RAID member ESPs...", "Writing redundant UEFI bootloaders to all drive ESPs")
		if err := syncMemberESPs(ctx, layout.MemberESPs, streamTarget); err != nil {
			slog.WarnContext(ctx, "non-fatal warning synchronizing member ESPs", "error", err)
		}
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

func syncMemberESPs(ctx context.Context, memberESPs []string, streamTarget string) error {
	rootPart, err := findRootPartition(ctx, streamTarget)
	if err != nil {
		return err
	}

	rootMount := "/mnt/redwolf-raid-sync"
	if err := os.MkdirAll(rootMount, 0755); err != nil {
		return err
	}
	if out, err := exec.CommandContext(ctx, "mount", "-o", "ro", rootPart, rootMount).CombinedOutput(); err != nil {
		return fmt.Errorf("failed mounting root for ESP sync: %w (output: %s)", err, string(out))
	}
	defer func() {
		_ = exec.CommandContext(context.Background(), "umount", rootMount).Run()
	}()

	var efiSrc string
	possiblePaths := []string{
		filepath.Join(rootMount, "boot", "efi", "EFI"),
		filepath.Join(rootMount, "EFI"),
	}
	for _, p := range possiblePaths {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			efiSrc = p
			break
		}
	}

	if efiSrc == "" {
		return fmt.Errorf("could not locate EFI directory in rootfs")
	}

	espMount := "/mnt/redwolf-member-esp"
	_ = os.MkdirAll(espMount, 0755)

	for _, espPart := range memberESPs {
		_ = exec.CommandContext(ctx, "mkfs.vfat", "-F32", espPart).Run()
		if out, err := exec.CommandContext(ctx, "mount", espPart, espMount).CombinedOutput(); err == nil {
			targetEFI := filepath.Join(espMount, "EFI")
			_ = os.MkdirAll(targetEFI, 0755)
			_ = exec.CommandContext(ctx, "cp", "-a", efiSrc+"/.", targetEFI+"/").Run()
			_ = exec.CommandContext(ctx, "umount", espMount).Run()
			slog.InfoContext(ctx, "synchronized redundant UEFI bootloader to RAID member ESP", "partition", espPart)
		} else {
			slog.WarnContext(ctx, "failed mounting member ESP", "partition", espPart, "output", string(out))
		}
	}
	return nil
}

// executeLVMDeployment mounts target LVM logical volumes, unpacks the distribution image,
// generates the LVM /etc/fstab, injects cloud-init, and configures the UEFI bootloader.
func executeLVMDeployment(ctx context.Context, task *domain.DeploymentTask, layout *StorageLayoutResult, bootMAC string, reporter ProgressReporter) error {
	nodeID := task.NodeID
	targetRootMount := "/mnt/redwolf-target"
	if err := os.MkdirAll(targetRootMount, 0755); err != nil {
		return fmt.Errorf("failed creating mount directory %s: %w", targetRootMount, err)
	}

	// 1. Mount root Logical Volume
	rootLV := layout.RootPartition
	if rootLV == "" {
		rootLV = "/dev/vg_system/root"
	}

	_ = reporter.Report(ctx, nodeID, 15, "Mounting LVM target hierarchy...", fmt.Sprintf("Mounting root volume %s to %s", rootLV, targetRootMount))
	if out, err := exec.CommandContext(ctx, "mount", rootLV, targetRootMount).CombinedOutput(); err != nil {
		return fmt.Errorf("failed mounting root LV %s: %w (output: %s)", rootLV, err, string(out))
	}

	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		unmountAllUnder(cleanupCtx, targetRootMount)
	}()

	// 2. Mount /boot and /boot/efi
	bootMount := filepath.Join(targetRootMount, "boot")
	_ = os.MkdirAll(bootMount, 0755)
	if out, err := exec.CommandContext(ctx, "mount", layout.BootPartition, bootMount).CombinedOutput(); err != nil {
		slog.WarnContext(ctx, "failed mounting boot partition", "partition", layout.BootPartition, "output", string(out))
	}

	efiMount := filepath.Join(bootMount, "efi")
	_ = os.MkdirAll(efiMount, 0755)
	if out, err := exec.CommandContext(ctx, "mount", layout.ESPPartition, efiMount).CombinedOutput(); err != nil {
		slog.WarnContext(ctx, "failed mounting efi partition", "partition", layout.ESPPartition, "output", string(out))
	}

	// 3. Mount additional Logical Volumes (e.g. /var, /home)
	var subMounts []lvmMountEntry
	for _, vol := range layout.LVMVolumes {
		mp := strings.TrimSpace(vol.MountPoint)
		if mp == "" || mp == "/" {
			continue
		}
		targetPath := filepath.Join(targetRootMount, mp)
		_ = os.MkdirAll(targetPath, 0755)
		lvDev := fmt.Sprintf("/dev/vg_system/%s", vol.Name)
		if out, err := exec.CommandContext(ctx, "mount", lvDev, targetPath).CombinedOutput(); err != nil {
			slog.WarnContext(ctx, "failed mounting sub volume", "lv", lvDev, "mount", targetPath, "output", string(out))
		}
		subMounts = append(subMounts, lvmMountEntry{
			device:     lvDev,
			mountPoint: mp,
			fsType:     vol.FSType,
		})
	}

	// 4. Stream and unpack OS image to sparse file
	_ = reporter.Report(ctx, nodeID, 25, "Streaming distribution image...", "Streaming and decompressing raw OS image to loop container")
	tmpImage := "/tmp/redwolf-cloud-image.raw"
	onProgress := func(pct int, writtenBytes int64, msg string) {
		scaled := 25 + int(float64(pct)*0.4) // Scaled between 25% and 65%
		_ = reporter.Report(ctx, nodeID, scaled, fmt.Sprintf("Streaming %s...", task.OS), msg)
	}

	if err := StreamImage(ctx, task.ImageURL, tmpImage, onProgress); err != nil {
		return fmt.Errorf("failed streaming image for LVM deployment: %w", err)
	}
	defer os.Remove(tmpImage)

	// 5. Attach loop device
	_ = reporter.Report(ctx, nodeID, 65, "Extracting distribution rootfs...", "Mounting source image loop container")
	loopCmd := exec.CommandContext(ctx, "losetup", "-P", "-r", "-f", "--show", tmpImage)
	loopOut, err := loopCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("losetup failed on %s: %w (output: %s)", tmpImage, err, string(loopOut))
	}
	loopDev := strings.TrimSpace(string(loopOut))
	defer func() {
		_ = exec.CommandContext(context.Background(), "losetup", "-d", loopDev).Run()
	}()

	_ = exec.CommandContext(ctx, "partprobe", loopDev).Run()
	time.Sleep(1 * time.Second)

	// 6. Copy files from source partitions into target LVM filesystem hierarchy
	if err := extractImageToLVM(ctx, loopDev, targetRootMount, reporter, nodeID, task.OS); err != nil {
		return fmt.Errorf("failed extracting source image to LVM target: %w", err)
	}

	// 7. Generate clean /etc/fstab for the new LVM layout
	_ = reporter.Report(ctx, nodeID, 80, "Generating LVM /etc/fstab...", "Writing filesystem table with device mapper and partition UUIDs")
	if err := generateLVMFstab(ctx, targetRootMount, layout, subMounts); err != nil {
		slog.WarnContext(ctx, "warning generating fstab for LVM", "error", err)
	}

	// 8. Inject Cloud-Init NoCloud seed
	_ = reporter.Report(ctx, nodeID, 85, "Injecting Cloud-Init NoCloud seed...", "Writing user-data, meta-data, and network-config")
	if err := WriteNoCloudSeeds(targetRootMount, task.Config, bootMAC); err != nil {
		return fmt.Errorf("failed injecting cloud-init into LVM rootfs: %w", err)
	}

	// 9. Unmount all target partitions
	_ = reporter.Report(ctx, nodeID, 90, "Finalizing storage writes...", "Flushing disk buffers and unmounting target volumes")
	unmountAllUnder(ctx, targetRootMount)

	// 10. Register UEFI boot entry
	_ = reporter.Report(ctx, nodeID, 95, "Registering UEFI boot entry...", "Configuring NVRAM with efibootmgr")
	bootDisk := layout.TargetDisk
	if bootDisk == "" {
		bootDisk = task.TargetDrivePath
	}
	if err := ConfigureBootloader(ctx, bootDisk, task.OS); err != nil {
		slog.WarnContext(ctx, "non-fatal warning during bootloader config", "drive", bootDisk, "error", err)
	}

	return nil
}

func extractImageToLVM(ctx context.Context, loopDev, targetRootMount string, reporter ProgressReporter, nodeID string, osType domain.OperatingSystem) error {
	// Source root detection
	sourceRootPart, err := findRootPartition(ctx, loopDev)
	if err != nil {
		sourceRootPart = fallbackPartitionPath(loopDev)
	}

	srcRootMount := "/mnt/redwolf-source-root"
	_ = os.MkdirAll(srcRootMount, 0755)
	if out, err := exec.CommandContext(ctx, "mount", "-o", "ro", sourceRootPart, srcRootMount).CombinedOutput(); err != nil {
		return fmt.Errorf("failed mounting source root %s: %w (output: %s)", sourceRootPart, err, string(out))
	}
	defer func() {
		_ = exec.CommandContext(context.Background(), "umount", srcRootMount).Run()
	}()

	_ = reporter.Report(ctx, nodeID, 70, "Synchronizing root filesystem...", "Copying operating system root files into LVM volumes")
	cpRootCmd := exec.CommandContext(ctx, "cp", "-a", srcRootMount+"/.", targetRootMount+"/")
	if out, err := cpRootCmd.CombinedOutput(); err != nil {
		slog.WarnContext(ctx, "cp rootfs warning", "error", err, "output", string(out))
	}

	// Source boot partition (partition 3 on AlmaLinux)
	srcBootPart := resolvePartitionPath(loopDev, 3)
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
	efiPartNum := detectEFIPartition(ctx, loopDev)
	srcEFIPart := resolvePartitionPath(loopDev, efiPartNum)
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

	return nil
}

type lvmMountEntry struct {
	device     string
	mountPoint string
	fsType     string
}

func generateLVMFstab(ctx context.Context, targetRootMount string, layout *StorageLayoutResult, subMounts []lvmMountEntry) error {
	fstabPath := filepath.Join(targetRootMount, "etc", "fstab")
	_ = os.MkdirAll(filepath.Dir(fstabPath), 0755)

	bootUUID := getPartitionUUID(ctx, layout.BootPartition)
	espUUID := getPartitionUUID(ctx, layout.ESPPartition)

	var sb strings.Builder
	sb.WriteString("# /etc/fstab: auto-generated by RedWolf Bare-Metal Provisioning Engine\n")
	sb.WriteString("# <file system> <mount point> <type> <options> <dump> <pass>\n")
	sb.WriteString("/dev/mapper/vg_system-root / xfs defaults 0 0\n")

	for _, sm := range subMounts {
		fs := sm.fsType
		if fs == "" {
			fs = "xfs"
		}
		name := filepath.Base(sm.device)
		sb.WriteString(fmt.Sprintf("/dev/mapper/vg_system-%s %s %s defaults 0 0\n", name, sm.mountPoint, fs))
	}

	if layout.SwapDevice != "" {
		sb.WriteString("/dev/mapper/vg_system-swap none swap sw 0 0\n")
	}

	if bootUUID != "" {
		sb.WriteString(fmt.Sprintf("UUID=%s /boot xfs defaults 0 0\n", bootUUID))
	} else if layout.BootPartition != "" {
		sb.WriteString(fmt.Sprintf("%s /boot xfs defaults 0 0\n", layout.BootPartition))
	}

	if espUUID != "" {
		sb.WriteString(fmt.Sprintf("UUID=%s /boot/efi vfat umask=0077,shortname=winnt 0 2\n", espUUID))
	} else if layout.ESPPartition != "" {
		sb.WriteString(fmt.Sprintf("%s /boot/efi vfat umask=0077,shortname=winnt 0 2\n", layout.ESPPartition))
	}

	return os.WriteFile(fstabPath, []byte(sb.String()), 0644)
}

func getPartitionUUID(ctx context.Context, partDev string) string {
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
	// Sort by descending length (deepest mounts first)
	for i := len(mounts) - 1; i >= 0; i-- {
		_ = exec.CommandContext(ctx, "umount", mounts[i]).Run()
	}
	_ = exec.CommandContext(ctx, "umount", rootMount).Run()
}

