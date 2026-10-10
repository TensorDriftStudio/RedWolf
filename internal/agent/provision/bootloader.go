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

// IsEFIFirmware reports whether the system firmware booted under UEFI.
func IsEFIFirmware() bool {
	_, err := os.Stat("/sys/firmware/efi")
	return err == nil
}

// ConfigureBootloader registers the UEFI bootloader in system NVRAM via efibootmgr.
func ConfigureBootloader(ctx context.Context, targetDrivePath string, osType domain.OperatingSystem, diskIndex ...int) error {
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
	if !IsEFIFirmware() {
		slog.WarnContext(ctx, "system is running in Legacy BIOS mode; EFI NVRAM variables are inaccessible (configure VM/system firmware to UEFI to enable native NVRAM boot)",
			"target_drive", targetDrivePath,
			"real_disk", realDisk,
		)
		return nil
	}

	// Ensure efivarfs is mounted for reliable NVRAM variable access
	_ = exec.CommandContext(ctx, "mount", "-t", "efivarfs", "efivarfs", "/sys/firmware/efi/efivars").Run()

	// Determine EFI loader path based on distribution
	loaderPath := `\EFI\BOOT\BOOTX64.EFI`
	switch osType {
	case domain.OSAlmaLinux8, domain.OSAlmaLinux9, domain.OSAlmaLinux10:
		loaderPath = `\EFI\almalinux\shimx64.efi`
	case domain.OSDebian12, domain.OSDebian13:
		loaderPath = `\EFI\debian\shimx64.efi`
	case domain.OSUbuntu2404, domain.OSUbuntu2204:
		loaderPath = `\EFI\ubuntu\shimx64.efi`
	}

	label := fmt.Sprintf("RedWolf (%s)", osType)
	if len(diskIndex) > 0 {
		label = fmt.Sprintf("RedWolf (%s) - Disk %d", osType, diskIndex[0])
	}
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
	defaultPart := 2
	if strings.Contains(realDisk, "loop") {
		if strings.Contains(strings.ToLower(string(targetOS)), "debian") || strings.Contains(strings.ToLower(string(targetOS)), "ubuntu") {
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
// and embeds core.img into the BIOS Boot Partition on target drives.
func InstallBIOSBootloader(ctx context.Context, targetDrives []string, bootMount, targetRootMount string, mode ...domain.FirmwareMode) error {
	if bootMount == "" && targetRootMount != "" {
		bootMount = filepath.Join(targetRootMount, "boot")
	}

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

		grubBin := ""
		if _, err := exec.LookPath("grub-install"); err == nil {
			grubBin = "grub-install"
		} else if _, err := exec.LookPath("grub2-install"); err == nil {
			grubBin = "grub2-install"
		}

		if grubBin != "" {
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

			cmd := exec.CommandContext(ctx, grubBin, argsWithModules...)
			out, err := cmd.CombinedOutput()
			if err == nil {
				slog.InfoContext(ctx, "successfully installed BIOS bootloader with pre-loaded modules", "disk", realDisk)
				installed = true
			} else {
				slog.DebugContext(ctx, "grub-install with modules returned error, retrying standard grub-install",
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
				fbCmd := exec.CommandContext(ctx, grubBin, fallbackArgs...)
				fbOut, fbErr := fbCmd.CombinedOutput()
				if fbErr == nil {
					slog.InfoContext(ctx, "successfully installed BIOS bootloader via fallback grub-install", "disk", realDisk)
					installed = true
				} else {
					slog.DebugContext(ctx, "grub-install failed", "disk", realDisk, "error", fbErr, "output", string(fbOut))
				}
			}
		}

		if !installed && targetRootMount != "" {
			binds := []string{"/dev", "/proc", "/sys", "/run"}
			if _, err := os.Stat("/dev/pts"); err == nil {
				binds = append(binds, "/dev/pts")
			}
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

		_ = exec.CommandContext(ctx, "parted", "-s", realDisk, "disk_set", "pmbr_boot", "on").Run()

		if !installed {
			if len(mode) > 0 && mode[0] == domain.FirmwareBIOS {
				return fmt.Errorf("%w: failed installing BIOS MBR bootloader onto %s (target image lacks grub-pc or bios_grub partition)", domain.ErrBootloaderFailed, realDisk)
			}
			slog.WarnContext(ctx, "could not execute grub-install for BIOS MBR (system will boot via UEFI or requires manual bootloader installation)", "disk", realDisk)
		}
	}
	return nil
}

// KernelInfo represents an identified kernel and its associated initramfs image.
type KernelInfo struct {
	KernelFile    string
	InitrdFile    string
	KernelVersion string
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

	sort.Slice(kernelCandidates, func(i, j int) bool {
		return kernelCandidates[i] > kernelCandidates[j]
	})

	selectedKernel := kernelCandidates[0]
	kVer := strings.TrimPrefix(selectedKernel, "vmlinuz-")

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
		vgName := layout.LVMVolumeGroup
		if vgName == "" {
			vgName = "vg_system"
		}
		rootLV := "root"
		for _, v := range layout.LVMVolumes {
			if v.MountPoint == "/" || v.Name == "root" {
				rootLV = v.Name
				break
			}
		}
		rootDev := layout.RootPartition
		if rootDev == "" {
			rootDev = fmt.Sprintf("/dev/mapper/%s-%s",
				strings.ReplaceAll(vgName, "-", "--"),
				strings.ReplaceAll(rootLV, "-", "--"),
			)
		}
		parts = append(parts,
			fmt.Sprintf("root=%s", rootDev),
			"rd.auto=1",
			"rd.lvm=1",
			fmt.Sprintf("rd.lvm.lv=%s/%s", vgName, rootLV),
		)
		if layout.IsSoftwareRAID {
			parts = append(parts, "rd.md=1")
			if layout.DataRAIDUUID != "" {
				parts = append(parts, fmt.Sprintf("rd.md.uuid=%s", layout.DataRAIDUUID))
			}
			if layout.BootRAIDUUID != "" {
				parts = append(parts, fmt.Sprintf("rd.md.uuid=%s", layout.BootRAIDUUID))
			}
		}
	} else if layout.IsSoftwareRAID {
		parts = append(parts, "rd.auto=1", "rd.md=1")
		if layout.DataRAIDUUID != "" {
			parts = append(parts, fmt.Sprintf("rd.md.uuid=%s", layout.DataRAIDUUID))
		}
		if layout.BootRAIDUUID != "" {
			parts = append(parts, fmt.Sprintf("rd.md.uuid=%s", layout.BootRAIDUUID))
		}
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

	parts = append(parts, "ro", "console=tty0", "console=ttyS0,115200n8", "plymouth.enable=0", "systemd.show_status=1")
	return strings.Join(parts, " ")
}

// GenerateUniversalGrubConfig inspects installed kernel and initramfs images and writes
// direct boot configurations across GRUB BIOS and EFI locations.
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

	// Ensure symlink /boot/boot -> . exists so /boot/vmlinuz paths resolve consistently
	bootSymlink := filepath.Join(bootDir, "boot")
	_ = os.Remove(bootSymlink)
	_ = os.Symlink(".", bootSymlink)

	newRootUUID := getPartitionUUID(ctx, layout.RootPartition)
	bootUUID := getPartitionUUID(ctx, layout.BootPartition)
	if bootUUID == "" {
		bootUUID = newRootUUID
	}

	fallbackPart := "hd0,gpt3"
	targetBootDev := layout.BootPartition
	if targetBootDev == "" {
		targetBootDev = layout.RootPartition
	}
	if strings.Contains(targetBootDev, "md") {
		fallbackPart = "md/0"
	} else if targetBootDev != "" {
		num := extractTrailingDigits(targetBootDev)
		if num > 0 {
			fallbackPart = fmt.Sprintf("hd0,gpt%d", num)
		}
	}

	kernelArgs := ComputeKernelArgs(layout, newRootUUID)
	distroLabel := fmt.Sprintf("%s", osType)

	var sb strings.Builder
	sb.WriteString("# Generated by RedWolf Bare-Metal Engine\n\n")
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
	sb.WriteString(fmt.Sprintf("if [ -z \"$root\" ]; then\n    set root=%s\nfi\n\n", fallbackPart))

	// Primary direct boot entry: checks if kernel is in /boot or root
	sb.WriteString(fmt.Sprintf("menuentry \"RedWolf (%s - Direct Boot)\" --class gnu-linux --class os {\n", distroLabel))
	sb.WriteString("    insmod part_gpt\n    insmod part_msdos\n    insmod ext2\n    insmod xfs\n    insmod mdraid1x\n    insmod lvm\n")
	if bootUUID != "" {
		sb.WriteString(fmt.Sprintf("    search --no-floppy --fs-uuid --set=root %s\n", bootUUID))
	} else {
		sb.WriteString("    search --no-floppy --label --set=root boot\n")
	}
	sb.WriteString(fmt.Sprintf("    if [ -z \"$root\" ]; then\n        set root=%s\n    fi\n", fallbackPart))
	sb.WriteString(fmt.Sprintf("    if [ -f /boot/%s ]; then\n", kInfo.KernelFile))
	sb.WriteString(fmt.Sprintf("        linux /boot/%s %s\n", kInfo.KernelFile, kernelArgs))
	sb.WriteString(fmt.Sprintf("        initrd /boot/%s\n", kInfo.InitrdFile))
	sb.WriteString("    else\n")
	sb.WriteString(fmt.Sprintf("        linux /%s %s\n", kInfo.KernelFile, kernelArgs))
	sb.WriteString(fmt.Sprintf("        initrd /%s\n", kInfo.InitrdFile))
	sb.WriteString("    fi\n")
	sb.WriteString("}\n\n")

	// Secondary direct boot entry with /boot prefix
	sb.WriteString(fmt.Sprintf("menuentry \"RedWolf (%s - Root Path /boot)\" --class gnu-linux --class os {\n", distroLabel))
	sb.WriteString("    insmod part_gpt\n    insmod part_msdos\n    insmod ext2\n    insmod xfs\n    insmod mdraid1x\n    insmod lvm\n")
	if bootUUID != "" {
		sb.WriteString(fmt.Sprintf("    search --no-floppy --fs-uuid --set=root %s\n", bootUUID))
	} else {
		sb.WriteString("    search --no-floppy --label --set=root boot\n")
	}
	sb.WriteString(fmt.Sprintf("    if [ -z \"$root\" ]; then\n        set root=%s\n    fi\n", fallbackPart))
	sb.WriteString(fmt.Sprintf("    linux /boot/%s %s\n", kInfo.KernelFile, kernelArgs))
	sb.WriteString(fmt.Sprintf("    initrd /boot/%s\n", kInfo.InitrdFile))
	sb.WriteString("}\n\n")

	if layout.IsSoftwareRAID {
		sb.WriteString(fmt.Sprintf("menuentry \"RedWolf (%s - RAID md0 Fallback)\" --class gnu-linux --class os {\n", distroLabel))
		sb.WriteString("    insmod part_gpt\n    insmod part_msdos\n    insmod ext2\n    insmod xfs\n    insmod mdraid1x\n    insmod lvm\n")
		sb.WriteString("    set root=md/0\n")
		sb.WriteString(fmt.Sprintf("    linux /%s %s\n", kInfo.KernelFile, kernelArgs))
		sb.WriteString(fmt.Sprintf("    initrd /%s\n", kInfo.InitrdFile))
		sb.WriteString("}\n\n")
	}

	// Partition fallback entry
	sb.WriteString(fmt.Sprintf("menuentry \"RedWolf (%s - Partition Fallback %s)\" --class gnu-linux --class os {\n", distroLabel, fallbackPart))
	sb.WriteString("    insmod part_gpt\n    insmod part_msdos\n    insmod ext2\n    insmod xfs\n    insmod mdraid1x\n    insmod lvm\n")
	sb.WriteString(fmt.Sprintf("    set root=%s\n", fallbackPart))
	sb.WriteString(fmt.Sprintf("    linux /%s %s\n", kInfo.KernelFile, kernelArgs))
	sb.WriteString(fmt.Sprintf("    initrd /%s\n", kInfo.InitrdFile))
	sb.WriteString("}\n")

	cfgData := []byte(sb.String())

	// Write grub.cfg to standard locations
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

	// Write EFI stub grub.cfg to ESP directories
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
			efiSb.WriteString(fmt.Sprintf("if [ -z \"$dev\" ]; then\n    set dev=%s\nfi\n\n", fallbackPart))

			efiSb.WriteString("configfile ($dev)/grub2/grub.cfg\n")
			efiSb.WriteString("configfile ($dev)/grub/grub.cfg\n")
			efiSb.WriteString("configfile ($dev)/boot/grub2/grub.cfg\n")
			efiSb.WriteString("configfile ($dev)/boot/grub/grub.cfg\n")
			efiSb.WriteString("configfile ($dev)/grub.cfg\n\n")

			efiSb.WriteString(fmt.Sprintf("menuentry \"RedWolf (%s - EFI Direct)\" {\n", distroLabel))
			if bootUUID != "" {
				efiSb.WriteString(fmt.Sprintf("    search --no-floppy --fs-uuid --set=root %s\n", bootUUID))
			} else {
				efiSb.WriteString("    search --no-floppy --label --set=root boot\n")
			}
			efiSb.WriteString(fmt.Sprintf("    if [ -z \"$root\" ]; then\n        set root=%s\n    fi\n", fallbackPart))
			efiSb.WriteString(fmt.Sprintf("    if [ -f ($root)/boot/%s ]; then\n", kInfo.KernelFile))
			efiSb.WriteString(fmt.Sprintf("        linux /boot/%s %s\n", kInfo.KernelFile, kernelArgs))
			efiSb.WriteString(fmt.Sprintf("        initrd /boot/%s\n", kInfo.InitrdFile))
			efiSb.WriteString("    else\n")
			efiSb.WriteString(fmt.Sprintf("        linux /%s %s\n", kInfo.KernelFile, kernelArgs))
			efiSb.WriteString(fmt.Sprintf("        initrd /%s\n", kInfo.InitrdFile))
			efiSb.WriteString("    fi\n")
			efiSb.WriteString("}\n")

			_ = os.WriteFile(efiCfgPath, []byte(efiSb.String()), 0644)
			slog.InfoContext(ctx, "wrote EFI stub grub.cfg", "path", efiCfgPath)
		}
	}

	return nil
}

// EnsureFallbackUEFILoader populates /boot/efi/EFI/BOOT with fallback BOOTX64.EFI and grub.cfg
// so UEFI firmware will boot even if NVRAM variables are reset or not persisted.
func EnsureFallbackUEFILoader(ctx context.Context, targetRootMount, bootUUID string) error {
	efiBase := filepath.Join(targetRootMount, "boot", "efi", "EFI")
	entries, err := os.ReadDir(efiBase)
	if err != nil {
		return nil
	}

	var shimPath, grubBinPath string
	for _, entry := range entries {
		if !entry.IsDir() || strings.EqualFold(entry.Name(), "BOOT") {
			continue
		}
		subDir := filepath.Join(efiBase, entry.Name())
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
	}

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
	if bootUUID != "" {
		var sb strings.Builder
		sb.WriteString("insmod part_gpt\ninsmod ext2\ninsmod xfs\ninsmod mdraid1x\ninsmod lvm\n")
		sb.WriteString(fmt.Sprintf("search --no-floppy --fs-uuid --set=dev %s\n", bootUUID))
		sb.WriteString("configfile ($dev)/grub2/grub.cfg\n")
		sb.WriteString("configfile ($dev)/grub/grub.cfg\n")
		sb.WriteString("configfile ($dev)/boot/grub2/grub.cfg\n")
		sb.WriteString("configfile ($dev)/boot/grub/grub.cfg\n")
		sb.WriteString("configfile ($dev)/grub.cfg\n")
		_ = os.WriteFile(fallbackCfg, []byte(sb.String()), 0644)
	}

	return nil
}

