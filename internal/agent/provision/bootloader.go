package provision

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

	// If system booted in Legacy BIOS mode, NVRAM EFI variables are inaccessible
	if _, err := os.Stat("/sys/firmware/efi"); os.IsNotExist(err) {
		slog.WarnContext(ctx, "system is running in Legacy BIOS mode; EFI NVRAM variables are inaccessible (configure VM/system firmware to UEFI to enable native NVRAM boot)",
			"target_drive", targetDrivePath,
			"real_disk", realDisk,
		)
		return nil
	}

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
	// Target disks partitioned by RedWolf place the EFI system partition at partition 2
	// (partition 1 is the 1MB BIOS boot partition).
	// Source cloud raw images attached via loopback devices have partition 2 (AlmaLinux) or 15 (Debian).
	defaultPart := 2
	if strings.Contains(realDisk, "loop") {
		if strings.Contains(strings.ToLower(string(targetOS)), "debian") {
			defaultPart = 15
		} else if strings.Contains(strings.ToLower(string(targetOS)), "almalinux") {
			defaultPart = 2
		}
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

// InstallBIOSBootloader installs the GRUB MBR stage 1 bootloader into Sector 0
// and embeds core.img into the BIOS Boot Partition (part 1) on all target drives.
func InstallBIOSBootloader(ctx context.Context, targetDrives []string, bootMount, targetRootMount string) error {
	for _, drive := range targetDrives {
		realDisk, err := filepath.EvalSymlinks(drive)
		if err != nil {
			realDisk = drive
		}

		slog.InfoContext(ctx, "installing legacy BIOS MBR bootloader onto drive",
			"target_drive", drive,
			"real_disk", realDisk,
		)

		var installed bool

		// Method 1: Discovery Agent native grub-install (packaged via grub-bios)
		if _, err := exec.LookPath("grub-install"); err == nil {
			// First try with pre-loaded enterprise storage & partition drivers directly in core.img
			argsWithModules := []string{
				"--target=i386-pc",
				"--recheck",
				"--force",
				"--modules=part_gpt part_msdos ext2 xfs mdraid1x lvm biosdisk",
			}
			if bootMount != "" {
				argsWithModules = append(argsWithModules, fmt.Sprintf("--boot-directory=%s", bootMount))
			}
			argsWithModules = append(argsWithModules, realDisk)

			cmd := exec.CommandContext(ctx, "grub-install", argsWithModules...)
			out, err := cmd.CombinedOutput()
			if err == nil {
				slog.InfoContext(ctx, "successfully installed BIOS bootloader with pre-loaded modules", "disk", realDisk)
				installed = true
			} else {
				slog.DebugContext(ctx, "agent grub-install with modules returned error, retrying standard grub-install",
					"disk", realDisk, "error", err, "output", string(out),
				)
				fallbackArgs := []string{
					"--target=i386-pc",
					"--recheck",
					"--force",
				}
				if bootMount != "" {
					fallbackArgs = append(fallbackArgs, fmt.Sprintf("--boot-directory=%s", bootMount))
				}
				fallbackArgs = append(fallbackArgs, realDisk)
				fbCmd := exec.CommandContext(ctx, "grub-install", fallbackArgs...)
				fbOut, fbErr := fbCmd.CombinedOutput()
				if fbErr == nil {
					slog.InfoContext(ctx, "successfully installed BIOS bootloader via fallback grub-install", "disk", realDisk)
					installed = true
				} else {
					slog.DebugContext(ctx, "agent standard grub-install failed", "disk", realDisk, "error", fbErr, "output", string(fbOut))
				}
			}
		}

		// Method 2: Target rootfs chroot grub2-install / grub-install
		if !installed && targetRootMount != "" {
			binds := []string{"/dev", "/proc", "/sys"}
			for _, b := range binds {
				targetB := filepath.Join(targetRootMount, b)
				_ = os.MkdirAll(targetB, 0755)
				_ = exec.CommandContext(ctx, "mount", "--bind", b, targetB).Run()
			}

			for _, binary := range []string{"grub2-install", "grub-install"} {
				chrootCmd := exec.CommandContext(ctx, "chroot", targetRootMount, binary, "--target=i386-pc", "--recheck", "--force", realDisk)
				out, err := chrootCmd.CombinedOutput()
				if err == nil {
					slog.InfoContext(ctx, "successfully installed BIOS bootloader via chroot", "binary", binary, "disk", realDisk)
					installed = true
					break
				} else {
					slog.DebugContext(ctx, "chroot install attempt failed", "binary", binary, "error", err, "output", string(out))
				}
			}

			for i := len(binds) - 1; i >= 0; i-- {
				_ = exec.CommandContext(context.Background(), "umount", filepath.Join(targetRootMount, binds[i])).Run()
			}
		}

		if !installed {
			slog.WarnContext(ctx, "could not execute grub-install for BIOS MBR (system will boot via UEFI or requires manual bootloader installation)", "disk", realDisk)
		}
	}
	return nil
}

// KernelInfo represents an identified kernel and its associated initramfs image.
type KernelInfo struct {
	KernelFile    string // Filename relative to /boot (e.g. "vmlinuz-5.14.0-427.el9.x86_64")
	InitrdFile    string // Filename relative to /boot (e.g. "initramfs-5.14.0-427.el9.x86_64.img")
	KernelVersion string // Extracted version string
}

// FindInstalledKernel inspects the boot directory and discovers the latest kernel and matching initramfs.
func FindInstalledKernel(bootDir string) (*KernelInfo, error) {
	entries, err := os.ReadDir(bootDir)
	if err != nil {
		return nil, fmt.Errorf("failed reading boot directory %s: %w", bootDir, err)
	}

	var kernelCandidates []string
	var initrdCandidates []string

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "vmlinuz-") {
			if !strings.Contains(name, "rescue") {
				kernelCandidates = append(kernelCandidates, name)
			}
		} else if strings.HasPrefix(name, "initramfs-") || strings.HasPrefix(name, "initrd.img-") || strings.HasPrefix(name, "initrd-") {
			if !strings.Contains(name, "rescue") {
				initrdCandidates = append(initrdCandidates, name)
			}
		}
	}

	// Fallback to rescue images if only rescue kernels were detected
	if len(kernelCandidates) == 0 {
		for _, e := range entries {
			if !e.IsDir() && strings.HasPrefix(e.Name(), "vmlinuz") {
				kernelCandidates = append(kernelCandidates, e.Name())
			}
		}
	}
	if len(initrdCandidates) == 0 {
		for _, e := range entries {
			if !e.IsDir() && (strings.HasPrefix(e.Name(), "initramfs") || strings.HasPrefix(e.Name(), "initrd")) {
				initrdCandidates = append(initrdCandidates, e.Name())
			}
		}
	}

	if len(kernelCandidates) == 0 {
		return nil, fmt.Errorf("no kernel (vmlinuz-*) found in %s", bootDir)
	}

	// Sort kernels descending so the latest / highest version is selected
	sort.Slice(kernelCandidates, func(i, j int) bool {
		return kernelCandidates[i] > kernelCandidates[j]
	})

	selectedKernel := kernelCandidates[0]
	kVer := strings.TrimPrefix(selectedKernel, "vmlinuz-")

	// Identify matching initramfs image
	var selectedInitrd string
	for _, initrd := range initrdCandidates {
		if strings.Contains(initrd, kVer) {
			selectedInitrd = initrd
			break
		}
	}
	if selectedInitrd == "" && len(initrdCandidates) > 0 {
		sort.Slice(initrdCandidates, func(i, j int) bool {
			return initrdCandidates[i] > initrdCandidates[j]
		})
		selectedInitrd = initrdCandidates[0]
	}

	if selectedInitrd == "" {
		return nil, fmt.Errorf("no matching initramfs found for kernel %s in %s", selectedKernel, bootDir)
	}

	return &KernelInfo{
		KernelFile:    selectedKernel,
		InitrdFile:    selectedInitrd,
		KernelVersion: kVer,
	}, nil
}

// ComputeKernelArgs generates the kernel cmdline arguments matching the deployed storage architecture.
func ComputeKernelArgs(layout *StorageLayoutResult, newRootUUID string) string {
	var parts []string

	if layout.IsLVM {
		parts = append(parts,
			"root=/dev/mapper/vg_system-root",
			"rd.auto=1",
			"rd.lvm=1",
		)
		if layout.IsSoftwareRAID {
			parts = append(parts, "rd.md=1")
		}
	} else if layout.IsSoftwareRAID {
		parts = append(parts, "rd.auto=1", "rd.md=1")
		if newRootUUID != "" {
			parts = append(parts, fmt.Sprintf("root=UUID=%s", newRootUUID))
		} else {
			parts = append(parts, fmt.Sprintf("root=%s", layout.RootPartition))
		}
	} else {
		if newRootUUID != "" {
			parts = append(parts, fmt.Sprintf("root=UUID=%s", newRootUUID))
		} else if layout.RootPartition != "" {
			parts = append(parts, fmt.Sprintf("root=%s", layout.RootPartition))
		} else {
			parts = append(parts, fmt.Sprintf("root=%s", layout.TargetDrive))
		}
	}

	// Use console=tty0 and disable plymouth to prevent hangs on virtual machines and show live boot status
	parts = append(parts, "ro", "console=tty0", "plymouth.enable=0", "systemd.show_status=1")
	return strings.Join(parts, " ")
}

// GenerateUniversalGrubConfig inspects the provisioned rootfs/boot directory, detects installed
// kernel/initramfs images, and writes standalone, bulletproof grub.cfg files across all standard locations.
// It generates explicit menu entries that boot directly, eliminating reliance on distribution-specific
// BLS modules or missing configuration files on BIOS and UEFI firmware.
func GenerateUniversalGrubConfig(ctx context.Context, targetRootMount string, layout *StorageLayoutResult, osType domain.OperatingSystem) error {
	bootDir := filepath.Join(targetRootMount, "boot")
	kInfo, err := FindInstalledKernel(bootDir)
	if err != nil {
		slog.WarnContext(ctx, "could not auto-detect kernel in /boot; checking rootfs", "error", err)
		kInfo, err = FindInstalledKernel(targetRootMount)
		if err != nil {
			return fmt.Errorf("failed locating kernel: %w", err)
		}
	}

	slog.InfoContext(ctx, "discovered installed kernel for universal bootloader configuration",
		"kernel", kInfo.KernelFile,
		"initrd", kInfo.InitrdFile,
		"version", kInfo.KernelVersion,
	)

	// Ensure symlink /boot/boot -> . exists so both /vmlinuz and /boot/vmlinuz paths resolve identically
	bootSymlink := filepath.Join(bootDir, "boot")
	_ = os.Remove(bootSymlink)
	_ = os.Symlink(".", bootSymlink)

	newRootUUID := getPartitionUUID(ctx, layout.RootPartition)
	bootUUID := getPartitionUUID(ctx, layout.BootPartition)
	if bootUUID == "" {
		bootUUID = newRootUUID
	}

	kernelArgs := ComputeKernelArgs(layout, newRootUUID)
	distroLabel := fmt.Sprintf("%s", osType)

	var sb strings.Builder
	sb.WriteString("# RedWolf Enterprise Bootloader Configuration\n")
	sb.WriteString("# Automatically generated for dual BIOS/UEFI, Software RAID & LVM\n\n")
	sb.WriteString("set default=\"0\"\n")
	sb.WriteString("set timeout=5\n\n")

	sb.WriteString("insmod part_gpt\n")
	sb.WriteString("insmod part_msdos\n")
	sb.WriteString("insmod ext2\n")
	sb.WriteString("insmod xfs\n")
	sb.WriteString("insmod mdraid1x\n")
	sb.WriteString("insmod lvm\n")
	sb.WriteString("insmod all_video\n")
	sb.WriteString("insmod gfxterm\n\n")

	if bootUUID != "" {
		sb.WriteString(fmt.Sprintf("search --no-floppy --fs-uuid --set=root %s\n", bootUUID))
	} else {
		sb.WriteString("search --no-floppy --label --set=root boot\n")
	}
	sb.WriteString("if [ -z \"$root\" ]; then\n    set root=hd0,gpt3\nfi\n\n")

	// Menuentry 1: Primary direct boot by UUID or label
	sb.WriteString(fmt.Sprintf("menuentry \"RedWolf (%s - Direct Boot)\" --class gnu-linux --class os {\n", distroLabel))
	sb.WriteString("    insmod part_gpt\n    insmod part_msdos\n    insmod ext2\n    insmod xfs\n    insmod mdraid1x\n    insmod lvm\n")
	if bootUUID != "" {
		sb.WriteString(fmt.Sprintf("    search --no-floppy --fs-uuid --set=root %s\n", bootUUID))
	} else {
		sb.WriteString("    search --no-floppy --label --set=root boot\n")
	}
	sb.WriteString("    if [ -z \"$root\" ]; then\n        set root=hd0,gpt3\n    fi\n")
	sb.WriteString(fmt.Sprintf("    linux /%s %s\n", kInfo.KernelFile, kernelArgs))
	sb.WriteString(fmt.Sprintf("    initrd /%s\n", kInfo.InitrdFile))
	sb.WriteString("}\n\n")

	// Menuentry 2: Direct boot with /boot prefix
	sb.WriteString(fmt.Sprintf("menuentry \"RedWolf (%s - Root Path /boot)\" --class gnu-linux --class os {\n", distroLabel))
	sb.WriteString("    insmod part_gpt\n    insmod part_msdos\n    insmod ext2\n    insmod xfs\n    insmod mdraid1x\n    insmod lvm\n")
	if bootUUID != "" {
		sb.WriteString(fmt.Sprintf("    search --no-floppy --fs-uuid --set=root %s\n", bootUUID))
	} else {
		sb.WriteString("    search --no-floppy --label --set=root boot\n")
	}
	sb.WriteString("    if [ -z \"$root\" ]; then\n        set root=hd0,gpt3\n    fi\n")
	sb.WriteString(fmt.Sprintf("    linux /boot/%s %s\n", kInfo.KernelFile, kernelArgs))
	sb.WriteString(fmt.Sprintf("    initrd /boot/%s\n", kInfo.InitrdFile))
	sb.WriteString("}\n\n")

	// Menuentry 3: RAID fallback md0
	if layout.IsSoftwareRAID {
		sb.WriteString(fmt.Sprintf("menuentry \"RedWolf (%s - RAID md0 Fallback)\" --class gnu-linux --class os {\n", distroLabel))
		sb.WriteString("    insmod part_gpt\n    insmod part_msdos\n    insmod ext2\n    insmod xfs\n    insmod mdraid1x\n    insmod lvm\n")
		sb.WriteString("    set root=md/0\n")
		sb.WriteString(fmt.Sprintf("    linux /%s %s\n", kInfo.KernelFile, kernelArgs))
		sb.WriteString(fmt.Sprintf("    initrd /%s\n", kInfo.InitrdFile))
		sb.WriteString("}\n\n")
	}

	// Menuentry 4: Partition fallback hd0,gpt3
	sb.WriteString(fmt.Sprintf("menuentry \"RedWolf (%s - Partition Fallback hd0,gpt3)\" --class gnu-linux --class os {\n", distroLabel))
	sb.WriteString("    insmod part_gpt\n    insmod part_msdos\n    insmod ext2\n    insmod xfs\n    insmod mdraid1x\n    insmod lvm\n")
	sb.WriteString("    set root=hd0,gpt3\n")
	sb.WriteString(fmt.Sprintf("    linux /%s %s\n", kInfo.KernelFile, kernelArgs))
	sb.WriteString(fmt.Sprintf("    initrd /%s\n", kInfo.InitrdFile))
	sb.WriteString("}\n")

	cfgData := []byte(sb.String())

	// Write universal grub.cfg to all standard bootloader search locations
	targetCfgPaths := []string{
		filepath.Join(bootDir, "grub", "grub.cfg"),
		filepath.Join(bootDir, "grub2", "grub.cfg"),
		filepath.Join(bootDir, "grub.cfg"),
	}

	for _, p := range targetCfgPaths {
		_ = os.MkdirAll(filepath.Dir(p), 0755)
		if err := os.WriteFile(p, cfgData, 0644); err != nil {
			slog.WarnContext(ctx, "failed writing grub.cfg to path", "path", p, "error", err)
		} else {
			slog.InfoContext(ctx, "successfully wrote universal grub.cfg", "path", p)
		}
	}

	// Symlink /etc/grub2.cfg and /etc/grub.cfg in target rootfs
	etcDir := filepath.Join(targetRootMount, "etc")
	_ = os.MkdirAll(etcDir, 0755)
	_ = os.Remove(filepath.Join(etcDir, "grub2.cfg"))
	_ = os.Remove(filepath.Join(etcDir, "grub.cfg"))
	_ = os.Symlink("../boot/grub2/grub.cfg", filepath.Join(etcDir, "grub2.cfg"))
	_ = os.Symlink("../boot/grub/grub.cfg", filepath.Join(etcDir, "grub.cfg"))

	// Write clean, syntax-error-free EFI stub grub.cfg to ESP directories
	efiBase := filepath.Join(targetRootMount, "boot", "efi", "EFI")
	if efiEntries, err := os.ReadDir(efiBase); err == nil {
		for _, entry := range efiEntries {
			if !entry.IsDir() {
				continue
			}
			efiDir := filepath.Join(efiBase, entry.Name())
			efiCfgPath := filepath.Join(efiDir, "grub.cfg")

			var efiSb strings.Builder
			efiSb.WriteString("insmod part_gpt\n")
			efiSb.WriteString("insmod ext2\n")
			efiSb.WriteString("insmod xfs\n")
			efiSb.WriteString("insmod mdraid1x\n")
			efiSb.WriteString("insmod lvm\n\n")

			if bootUUID != "" {
				efiSb.WriteString(fmt.Sprintf("search --no-floppy --fs-uuid --set=dev %s\n", bootUUID))
			} else {
				efiSb.WriteString("search --no-floppy --label --set=dev boot\n")
			}
			efiSb.WriteString("if [ -z \"$dev\" ]; then\n    set dev=hd0,gpt3\nfi\n\n")

			efiSb.WriteString("configfile ($dev)/grub2/grub.cfg\n")
			efiSb.WriteString("configfile ($dev)/grub/grub.cfg\n")
			efiSb.WriteString("configfile ($dev)/boot/grub2/grub.cfg\n")
			efiSb.WriteString("configfile ($dev)/boot/grub/grub.cfg\n")
			efiSb.WriteString("configfile ($dev)/grub.cfg\n\n")

			// Direct fallback menuentry in case configfile chainload encounters an error
			efiSb.WriteString(fmt.Sprintf("menuentry \"RedWolf (%s - EFI Direct)\" {\n", distroLabel))
			if bootUUID != "" {
				efiSb.WriteString(fmt.Sprintf("    search --no-floppy --fs-uuid --set=root %s\n", bootUUID))
			} else {
				efiSb.WriteString("    search --no-floppy --label --set=root boot\n")
			}
			efiSb.WriteString("    if [ -z \"$root\" ]; then\n        set root=hd0,gpt3\n    fi\n")
			efiSb.WriteString(fmt.Sprintf("    linux /%s %s\n", kInfo.KernelFile, kernelArgs))
			efiSb.WriteString(fmt.Sprintf("    initrd /%s\n", kInfo.InitrdFile))
			efiSb.WriteString("}\n")

			_ = os.WriteFile(efiCfgPath, []byte(efiSb.String()), 0644)
			slog.InfoContext(ctx, "wrote EFI stub grub.cfg", "path", efiCfgPath)
		}
	}

	return nil
}
