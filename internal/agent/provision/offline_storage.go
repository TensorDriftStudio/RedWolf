package provision

import (
	"context"
	"debug/elf"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// EnsureOfflineStorageTools guarantees that LVM and MDADM userspace tools,
// Dracut modules, and initramfs hooks are natively present in the target filesystem,
// even when the machine has zero internet connectivity.
func EnsureOfflineStorageTools(ctx context.Context, targetRootMount string, isSoftwareRAID, isLVM bool) error {
	slog.InfoContext(ctx, "evaluating and ensuring offline storage tools in target rootfs",
		"target_root", targetRootMount,
		"software_raid", isSoftwareRAID,
		"lvm", isLVM,
	)

	if isLVM {
		if err := InjectOfflineLVM(ctx, targetRootMount); err != nil {
			slog.WarnContext(ctx, "warning injecting offline lvm tools", "error", err)
		}
	}

	if isSoftwareRAID {
		if err := InjectOfflineMDADM(ctx, targetRootMount); err != nil {
			slog.WarnContext(ctx, "warning injecting offline mdadm tools", "error", err)
		}
	}

	if isLVM || isSoftwareRAID {
		if err := InjectDracutStorageModule(ctx, targetRootMount, isSoftwareRAID, isLVM); err != nil {
			slog.WarnContext(ctx, "warning configuring dracut storage modules", "error", err)
		}
		if err := InjectInitramfsToolsStorageHooks(ctx, targetRootMount, isSoftwareRAID, isLVM); err != nil {
			slog.WarnContext(ctx, "warning configuring initramfs-tools hooks", "error", err)
		}
		if err := InjectSystemdStorageServices(ctx, targetRootMount, isSoftwareRAID, isLVM); err != nil {
			slog.WarnContext(ctx, "warning configuring systemd storage service", "error", err)
		}
	}

	return nil
}

// InjectOfflineLVM verifies or injects LVM userspace binaries and tool symlinks into the target rootfs.
func InjectOfflineLVM(ctx context.Context, targetRootMount string) error {
	targetLVM := filepath.Join(targetRootMount, "usr", "sbin", "lvm")
	targetLVMAlt := filepath.Join(targetRootMount, "sbin", "lvm")

	if fi, err := os.Stat(targetLVM); err == nil && !fi.IsDir() && fi.Mode()&0111 != 0 {
		slog.DebugContext(ctx, "target rootfs already has working lvm binary in /usr/sbin/lvm")
		ensureLVMSymlinks(targetRootMount)
		return nil
	}
	if fi, err := os.Stat(targetLVMAlt); err == nil && !fi.IsDir() && fi.Mode()&0111 != 0 {
		slog.DebugContext(ctx, "target rootfs already has working lvm binary in /sbin/lvm")
		ensureLVMSymlinks(targetRootMount)
		return nil
	}

	// Locate host/agent source binary
	srcCandidates := []string{
		"/usr/sbin/lvm.static",
		"/sbin/lvm.static",
		"/sbin/lvm",
		"/usr/sbin/lvm",
		"/bin/lvm",
	}

	var foundSrc string
	for _, cand := range srcCandidates {
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() && fi.Mode()&0111 != 0 {
			foundSrc = cand
			break
		}
	}

	if foundSrc == "" {
		slog.WarnContext(ctx, "no source lvm binary found in discovery host environment to inject")
		return nil
	}

	slog.InfoContext(ctx, "injecting offline lvm binary into target rootfs", "source", foundSrc, "target", targetLVM)

	if err := copyExecutableFile(foundSrc, targetLVM); err != nil {
		return fmt.Errorf("failed copying lvm binary to %s: %w", targetLVM, err)
	}

	// Safely mirror to /sbin/lvm without corrupting merged-usr
	if err := mirrorBinary(targetLVM, targetLVMAlt); err != nil {
		slog.WarnContext(ctx, "warning mirroring lvm to /sbin/lvm", "error", err)
	}

	// Copy dynamic interpreter and ELF library dependencies if dynamically linked
	if err := copyELFDependencies(foundSrc, targetRootMount); err != nil {
		slog.WarnContext(ctx, "warning resolving ELF dependencies for lvm", "error", err)
	}

	ensureLVMSymlinks(targetRootMount)
	_ = ConfigureTargetLVM(targetRootMount)

	return nil
}

// InjectOfflineMDADM verifies or injects mdadm userspace binaries into the target rootfs.
func InjectOfflineMDADM(ctx context.Context, targetRootMount string) error {
	targetMdadm := filepath.Join(targetRootMount, "usr", "sbin", "mdadm")
	targetMdadmAlt := filepath.Join(targetRootMount, "sbin", "mdadm")

	if fi, err := os.Stat(targetMdadm); err == nil && !fi.IsDir() && fi.Mode()&0111 != 0 {
		slog.DebugContext(ctx, "target rootfs already has working mdadm binary in /usr/sbin/mdadm")
		return nil
	}
	if fi, err := os.Stat(targetMdadmAlt); err == nil && !fi.IsDir() && fi.Mode()&0111 != 0 {
		slog.DebugContext(ctx, "target rootfs already has working mdadm binary in /sbin/mdadm")
		return nil
	}

	srcCandidates := []string{
		"/usr/sbin/mdadm.static",
		"/sbin/mdadm.static",
		"/sbin/mdadm",
		"/usr/sbin/mdadm",
		"/bin/mdadm",
	}

	var foundSrc string
	for _, cand := range srcCandidates {
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() && fi.Mode()&0111 != 0 {
			foundSrc = cand
			break
		}
	}

	if foundSrc == "" {
		slog.WarnContext(ctx, "no source mdadm binary found in discovery host environment to inject")
		return nil
	}

	slog.InfoContext(ctx, "injecting offline mdadm binary into target rootfs", "source", foundSrc, "target", targetMdadm)

	if err := copyExecutableFile(foundSrc, targetMdadm); err != nil {
		return fmt.Errorf("failed copying mdadm binary to %s: %w", targetMdadm, err)
	}

	// Safely mirror to /sbin/mdadm without corrupting merged-usr
	if err := mirrorBinary(targetMdadm, targetMdadmAlt); err != nil {
		slog.WarnContext(ctx, "warning mirroring mdadm to /sbin/mdadm", "error", err)
	}

	// Copy dynamic interpreter and ELF library dependencies if dynamically linked
	if err := copyELFDependencies(foundSrc, targetRootMount); err != nil {
		slog.WarnContext(ctx, "warning resolving ELF dependencies for mdadm", "error", err)
	}

	return nil
}

// mirrorBinary guarantees that an executable in primary (e.g. /usr/sbin/foo)
// is also available at alt (e.g. /sbin/foo), while safely handling merged-/usr
// where alt's parent directory is already a symlink pointing to primary's parent directory.
func mirrorBinary(targetPrimary, targetAlt string) error {
	altDir := filepath.Dir(targetAlt)
	// Check if altDir itself is a symlink (e.g. target/sbin -> usr/sbin in merged-usr)
	if fi, err := os.Lstat(altDir); err == nil && (fi.Mode()&os.ModeSymlink != 0) {
		// In merged-usr systems, writing to /usr/sbin makes it immediately accessible via /sbin!
		// DO NOT remove or overwrite targetAlt, as doing so would delete targetPrimary.
		return nil
	}

	// Check if targetAlt and targetPrimary are already the exact same file
	fiPrimary, errPrimary := os.Stat(targetPrimary)
	if errPrimary == nil {
		if fiAlt, errAlt := os.Stat(targetAlt); errAlt == nil && os.SameFile(fiPrimary, fiAlt) {
			return nil
		}
	}

	// For traditional unmerged systems where altDir is an independent directory:
	_ = os.MkdirAll(altDir, 0755)
	_ = os.Remove(targetAlt)
	return copyExecutableFile(targetPrimary, targetAlt)
}

// ensureLVMSymlinks guarantees that all standard LVM volume management commands
// (vgchange, lvscan, pvcreate, etc.) are available as symlinks to lvm.
func ensureLVMSymlinks(targetRootMount string) {
	commands := []string{
		"vgscan", "vgchange", "vgs", "vgdisplay", "vgcreate", "vgremove", "vgextend", "vgreduce",
		"lvscan", "lvchange", "lvs", "lvdisplay", "lvcreate", "lvremove", "lvextend", "lvreduce", "lvresize",
		"pvscan", "pvchange", "pvs", "pvdisplay", "pvcreate", "pvremove",
	}

	dirs := []string{
		filepath.Join(targetRootMount, "usr", "sbin"),
		filepath.Join(targetRootMount, "sbin"),
	}

	for _, d := range dirs {
		if fi, err := os.Lstat(d); err == nil && (fi.Mode()&os.ModeSymlink != 0) {
			// Skip symlinked directory (merged-usr) since primary is already populated
			continue
		}
		_ = os.MkdirAll(d, 0755)
		for _, cmd := range commands {
			targetSymlink := filepath.Join(d, cmd)
			if _, err := os.Lstat(targetSymlink); err == nil {
				// Already exists (symlink or file)
				continue
			}
			_ = os.Symlink("lvm", targetSymlink)
		}
	}
}

// copyExecutableFile copies a file and marks it executable (0755).
func copyExecutableFile(src, dst string) error {
	_ = os.MkdirAll(filepath.Dir(dst), 0755)

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	_ = os.Remove(dst)
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// copyELFDependencies inspects an ELF executable, extracts required PT_INTERP program
// interpreters (e.g. musl loader) and DT_NEEDED shared libraries, and copies them to the target rootfs.
func copyELFDependencies(srcBinary, targetRootMount string) error {
	ef, err := elf.Open(srcBinary)
	if err != nil {
		// Not an ELF binary or static script; nothing to copy
		return nil
	}
	defer ef.Close()

	// 1. Program interpreter (PT_INTERP)
	for _, prog := range ef.Progs {
		if prog.Type == elf.PT_INTERP {
			interpBuf := make([]byte, prog.Filesz)
			if _, err := prog.ReadAt(interpBuf, 0); err == nil {
				interpPath := strings.TrimRight(string(interpBuf), "\x00 \t\n\r")
				if interpPath != "" {
					if _, err := os.Stat(interpPath); err == nil {
						dstPath := filepath.Join(targetRootMount, interpPath)
						_ = copyExecutableFile(interpPath, dstPath)

						// Also ensure /lib64 alias if under /lib
						if strings.HasPrefix(interpPath, "/lib/") {
							lib64Dir := filepath.Join(targetRootMount, "lib64")
							if fi, err := os.Lstat(lib64Dir); err == nil && (fi.Mode()&os.ModeSymlink != 0) {
								// In merged systems where lib64 is a symlink, dstPath is already accessible
							} else {
								lib64Path := filepath.Join(lib64Dir, filepath.Base(interpPath))
								_ = os.MkdirAll(lib64Dir, 0755)
								_ = os.Remove(lib64Path)
								_ = copyExecutableFile(dstPath, lib64Path)
							}
						}
					}
				}
			}
			break
		}
	}

	// 2. DT_NEEDED shared libraries
	libs, err := ef.ImportedLibraries()
	if err != nil || len(libs) == 0 {
		return nil
	}

	searchDirs := []string{
		"/lib",
		"/usr/lib",
		"/lib64",
		"/usr/lib64",
		"/usr/local/lib",
	}

	for _, lib := range libs {
		var foundLib string
		for _, dir := range searchDirs {
			cand := filepath.Join(dir, lib)
			if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
				foundLib = cand
				break
			}
		}
		if foundLib == "" {
			continue
		}

		// Determine relative directory
		relDir := filepath.Dir(foundLib)
		dstLib := filepath.Join(targetRootMount, relDir, lib)
		_ = copyExecutableFile(foundLib, dstLib)

		// If foundLib is a symlink, also resolve and copy real file
		if realTarget, err := filepath.EvalSymlinks(foundLib); err == nil && realTarget != foundLib {
			realDst := filepath.Join(targetRootMount, filepath.Dir(realTarget), filepath.Base(realTarget))
			_ = copyExecutableFile(realTarget, realDst)
		}
	}

	return nil
}

// InjectDracutStorageModule installs native and fallback dracut configurations into target rootfs.
func InjectDracutStorageModule(ctx context.Context, targetRootMount string, isSoftwareRAID, isLVM bool) error {
	dracutConfDir := filepath.Join(targetRootMount, "etc", "dracut.conf.d")
	_ = os.MkdirAll(dracutConfDir, 0755)

	var modules []string
	if isLVM {
		modules = append(modules, "lvm")
	}
	if isSoftwareRAID {
		modules = append(modules, "mdraid")
	}
	modules = append(modules, "redwolf-storage")

	confContent := fmt.Sprintf(`# Generated by RedWolf Bare-Metal Provisioning Engine (Offline Resilience)
add_dracutmodules+=" %s "
add_drivers+=" nvme megaraid_sas mpt3sas smartpqi ahci sd_mod raid0 raid1 raid10 dm_mod dm-mod "
`, strings.Join(modules, " "))

	confFile := filepath.Join(dracutConfDir, "99-redwolf-storage.conf")
	if err := os.WriteFile(confFile, []byte(confContent), 0644); err != nil {
		return fmt.Errorf("failed writing dracut config %s: %w", confFile, err)
	}

	// Provide bulletproof 90redwolf-storage fallback module in Dracut modules directory
	redwolfModDir := filepath.Join(targetRootMount, "usr", "lib", "dracut", "modules.d", "90redwolf-storage")
	_ = os.MkdirAll(redwolfModDir, 0755)

	setupScript := `#!/bin/bash
# RedWolf Offline Storage Auto-Assembly Dracut Module
check() { return 0; }
depends() { return 0; }
installkernel() {
    instmods nvme megaraid_sas mpt3sas smartpqi ahci sd_mod raid0 raid1 raid10 dm_mod dm-mod
}
install() {
    inst_multiple -o mdadm lvm /lib/ld-musl-x86_64.so.1 /lib64/ld-musl-x86_64.so.1
    [ -f /etc/mdadm.conf ] && inst_simple /etc/mdadm.conf
    [ -f /etc/mdadm/mdadm.conf ] && inst_simple /etc/mdadm/mdadm.conf
    [ -f /etc/lvm/lvm.conf ] && inst_simple /etc/lvm/lvm.conf
    [ -f /etc/lvm/lvmlocal.conf ] && inst_simple /etc/lvm/lvmlocal.conf
    inst_hook cmdline 30 "$moddir/redwolf-assemble.sh"
    inst_hook pre-mount 30 "$moddir/redwolf-assemble.sh"
}
`
	_ = os.WriteFile(filepath.Join(redwolfModDir, "module-setup.sh"), []byte(setupScript), 0755)

	assembleScript := `#!/bin/sh
# RedWolf Offline Storage Auto-Assembly Early Boot Hook
command -v mdadm >/dev/null 2>&1 && mdadm --assemble --scan --auto=yes || true
command -v lvm >/dev/null 2>&1 && lvm vgchange -ay || true
`
	_ = os.WriteFile(filepath.Join(redwolfModDir, "redwolf-assemble.sh"), []byte(assembleScript), 0755)

	slog.InfoContext(ctx, "dracut offline storage auto-assembly modules successfully configured", "path", confFile)
	return nil
}

// InjectInitramfsToolsStorageHooks configures hooks and pre-mount scripts for Debian / Ubuntu initramfs-tools.
func InjectInitramfsToolsStorageHooks(ctx context.Context, targetRootMount string, isSoftwareRAID, isLVM bool) error {
	initramfsDir := filepath.Join(targetRootMount, "etc", "initramfs-tools")
	if _, err := os.Stat(initramfsDir); os.IsNotExist(err) {
		// Target system does not use initramfs-tools
		return nil
	}

	// 1. Storage Drivers in /etc/initramfs-tools/modules
	modulesFile := filepath.Join(initramfsDir, "modules")
	var existingContent string
	if data, err := os.ReadFile(modulesFile); err == nil {
		existingContent = string(data)
	}

	requiredDrivers := []string{
		"nvme", "megaraid_sas", "mpt3sas", "smartpqi", "ahci", "sd_mod",
		"raid0", "raid1", "raid10", "dm-mod", "dm_mod",
	}

	var sb strings.Builder
	sb.WriteString(existingContent)
	if !strings.HasSuffix(existingContent, "\n") && existingContent != "" {
		sb.WriteString("\n")
	}
	for _, drv := range requiredDrivers {
		if !strings.Contains(existingContent, drv) {
			sb.WriteString(drv)
			sb.WriteString("\n")
		}
	}
	_ = os.WriteFile(modulesFile, []byte(sb.String()), 0644)

	// 2. Hook in /etc/initramfs-tools/hooks/redwolf-storage
	hooksDir := filepath.Join(initramfsDir, "hooks")
	_ = os.MkdirAll(hooksDir, 0755)

	hookScript := `#!/bin/sh
PREREQ=""
prereqs() { echo "$PREREQ"; }
case $1 in prereqs) prereqs; exit 0;; esac
. /usr/share/initramfs-tools/hook-functions

for b in /usr/sbin/mdadm /sbin/mdadm /usr/sbin/lvm /sbin/lvm; do
    if [ -x "$b" ]; then
        copy_exec "$b" "/sbin/$(basename "$b")"
    fi
done

if [ -f /lib/ld-musl-x86_64.so.1 ]; then
    copy_exec /lib/ld-musl-x86_64.so.1 /lib/ld-musl-x86_64.so.1
fi

if [ -x "${DESTDIR}/sbin/lvm" ]; then
    for cmd in vgscan vgchange vgs vgdisplay lvscan lvchange lvs pvscan pvchange pvs; do
        [ ! -e "${DESTDIR}/sbin/${cmd}" ] && ln -s lvm "${DESTDIR}/sbin/${cmd}"
    done
fi

[ -f /etc/mdadm.conf ] && cp -p /etc/mdadm.conf "${DESTDIR}/etc/mdadm.conf"
[ -f /etc/mdadm/mdadm.conf ] && mkdir -p "${DESTDIR}/etc/mdadm" && cp -p /etc/mdadm/mdadm.conf "${DESTDIR}/etc/mdadm/mdadm.conf"
[ -f /etc/lvm/lvm.conf ] && mkdir -p "${DESTDIR}/etc/lvm" && cp -p /etc/lvm/lvm.conf "${DESTDIR}/etc/lvm/lvm.conf"
[ -f /etc/lvm/lvmlocal.conf ] && mkdir -p "${DESTDIR}/etc/lvm" && cp -p /etc/lvm/lvmlocal.conf "${DESTDIR}/etc/lvm/lvmlocal.conf"
`
	hookPath := filepath.Join(hooksDir, "redwolf-storage")
	_ = os.WriteFile(hookPath, []byte(hookScript), 0755)

	// 3. Local-top script in /etc/initramfs-tools/scripts/local-top/redwolf-storage
	localTopDir := filepath.Join(initramfsDir, "scripts", "local-top")
	_ = os.MkdirAll(localTopDir, 0755)

	localTopScript := `#!/bin/sh
PREREQ=""
prereqs() { echo "$PREREQ"; }
case $1 in prereqs) prereqs; exit 0;; esac

if [ -x /sbin/mdadm ]; then
    /sbin/mdadm --assemble --scan --auto=yes || true
elif [ -x /usr/sbin/mdadm ]; then
    /usr/sbin/mdadm --assemble --scan --auto=yes || true
fi

if [ -x /sbin/lvm ]; then
    /sbin/lvm vgchange -ay || true
elif [ -x /usr/sbin/lvm ]; then
    /usr/sbin/lvm vgchange -ay || true
fi
`
	localTopPath := filepath.Join(localTopDir, "redwolf-storage")
	_ = os.WriteFile(localTopPath, []byte(localTopScript), 0755)

	slog.InfoContext(ctx, "initramfs-tools offline storage auto-assembly hooks successfully configured",
		"hook", hookPath,
		"local_top", localTopPath,
	)
	return nil
}

// InjectSystemdStorageServices creates a robust early systemd unit ensuring MDADM and LVM
// volumes are fully activated before local filesystems are mounted in the booted OS.
func InjectSystemdStorageServices(ctx context.Context, targetRootMount string, isSoftwareRAID, isLVM bool) error {
	systemdDir := filepath.Join(targetRootMount, "etc", "systemd", "system")
	_ = os.MkdirAll(systemdDir, 0755)

	unitContent := `[Unit]
Description=RedWolf Bare-Metal Storage Auto-Assembly (MDADM and LVM)
DefaultDependencies=no
After=systemd-udev-settle.service
Before=local-fs-pre.target
Wants=systemd-udev-settle.service
ConditionPathExists=/etc/redwolf-release

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=-/usr/sbin/mdadm --assemble --scan --auto=yes
ExecStart=-/usr/sbin/lvm vgchange -ay
TimeoutSec=30

[Install]
WantedBy=local-fs-pre.target
`
	unitFile := filepath.Join(systemdDir, "redwolf-storage-init.service")
	if err := os.WriteFile(unitFile, []byte(unitContent), 0644); err != nil {
		return fmt.Errorf("failed writing systemd unit %s: %w", unitFile, err)
	}

	wantsDir := filepath.Join(systemdDir, "local-fs-pre.target.wants")
	_ = os.MkdirAll(wantsDir, 0755)
	wantsSymlink := filepath.Join(wantsDir, "redwolf-storage-init.service")
	_ = os.Remove(wantsSymlink)
	_ = os.Symlink("../redwolf-storage-init.service", wantsSymlink)

	slog.InfoContext(ctx, "systemd early storage auto-assembly unit enabled", "unit", unitFile)
	return nil
}
