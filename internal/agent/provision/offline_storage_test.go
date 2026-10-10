package provision

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureOfflineStorageTools_DracutAndSystemd(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()

	err := EnsureOfflineStorageTools(ctx, tempDir, true, true)
	if err != nil {
		t.Fatalf("unexpected error ensuring offline storage tools: %v", err)
	}

	// 1. Verify Dracut Configuration
	dracutConf := filepath.Join(tempDir, "etc", "dracut.conf.d", "99-redwolf-storage.conf")
	data, err := os.ReadFile(dracutConf)
	if err != nil {
		t.Fatalf("expected dracut config at %s, error: %v", dracutConf, err)
	}
	content := string(data)
	if !strings.Contains(content, "add_dracutmodules+=") || !strings.Contains(content, "lvm") || !strings.Contains(content, "mdraid") {
		t.Errorf("dracut config missing required modules, got:\n%s", content)
	}
	if !strings.Contains(content, "add_drivers+=") || !strings.Contains(content, "dm-mod") || !strings.Contains(content, "raid1") {
		t.Errorf("dracut config missing required storage drivers, got:\n%s", content)
	}

	// 2. Verify Fallback Dracut Module
	modSetup := filepath.Join(tempDir, "usr", "lib", "dracut", "modules.d", "90redwolf-storage", "module-setup.sh")
	modData, err := os.ReadFile(modSetup)
	if err != nil {
		t.Fatalf("expected dracut module-setup.sh at %s, error: %v", modSetup, err)
	}
	if !strings.Contains(string(modData), "inst_multiple -o mdadm lvm") {
		t.Errorf("module-setup.sh missing inst_multiple call, got:\n%s", string(modData))
	}

	fi, err := os.Stat(modSetup)
	if err != nil || fi.Mode()&0111 == 0 {
		t.Errorf("expected module-setup.sh to be executable, got mode: %v", fi.Mode())
	}

	assembleHook := filepath.Join(tempDir, "usr", "lib", "dracut", "modules.d", "90redwolf-storage", "redwolf-assemble.sh")
	if fi2, err2 := os.Stat(assembleHook); err2 != nil || fi2.Mode()&0111 == 0 {
		t.Errorf("expected redwolf-assemble.sh to be executable, got error: %v", err2)
	}

	// 3. Verify Systemd Storage Service
	unitFile := filepath.Join(tempDir, "etc", "systemd", "system", "redwolf-storage-init.service")
	unitData, err := os.ReadFile(unitFile)
	if err != nil {
		t.Fatalf("expected systemd unit at %s, error: %v", unitFile, err)
	}
	unitStr := string(unitData)
	if !strings.Contains(unitStr, "ExecStart=-/usr/sbin/mdadm --assemble --scan") || !strings.Contains(unitStr, "ExecStart=-/usr/sbin/lvm vgchange -ay") {
		t.Errorf("systemd unit missing assembly commands, got:\n%s", unitStr)
	}

	// Verify Symlink in local-fs-pre.target.wants
	symlinkPath := filepath.Join(tempDir, "etc", "systemd", "system", "local-fs-pre.target.wants", "redwolf-storage-init.service")
	target, err := os.Readlink(symlinkPath)
	if err != nil {
		t.Fatalf("expected symlink at %s, error: %v", symlinkPath, err)
	}
	if target != "../redwolf-storage-init.service" {
		t.Errorf("expected symlink target ../redwolf-storage-init.service, got: %s", target)
	}
}

func TestInjectInitramfsToolsStorageHooks(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()

	// Simulate Debian / Ubuntu environment by creating /etc/initramfs-tools
	initramfsDir := filepath.Join(tempDir, "etc", "initramfs-tools")
	_ = os.MkdirAll(initramfsDir, 0755)
	_ = os.WriteFile(filepath.Join(initramfsDir, "modules"), []byte("# Initial modules\ne1000e\n"), 0644)

	err := InjectInitramfsToolsStorageHooks(ctx, tempDir, true, true)
	if err != nil {
		t.Fatalf("unexpected error injecting initramfs-tools hooks: %v", err)
	}

	// 1. Check Modules File
	modContent, err := os.ReadFile(filepath.Join(initramfsDir, "modules"))
	if err != nil {
		t.Fatalf("failed reading modules file: %v", err)
	}
	modStr := string(modContent)
	for _, expectedMod := range []string{"nvme", "raid1", "raid0", "dm-mod"} {
		if !strings.Contains(modStr, expectedMod) {
			t.Errorf("expected module %s in /etc/initramfs-tools/modules, got:\n%s", expectedMod, modStr)
		}
	}

	// 2. Check Hook Script
	hookPath := filepath.Join(initramfsDir, "hooks", "redwolf-storage")
	hookData, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("failed reading hook script %s: %v", hookPath, err)
	}
	if !strings.Contains(string(hookData), "copy_exec") || !strings.Contains(string(hookData), "lvm") || !strings.Contains(string(hookData), "mdadm") {
		t.Errorf("hook script missing copy_exec calls, got:\n%s", string(hookData))
	}
	fi, _ := os.Stat(hookPath)
	if fi.Mode()&0111 == 0 {
		t.Errorf("expected hook script to be executable (0755), got: %v", fi.Mode())
	}

	// 3. Check Local-Top Script
	localTopPath := filepath.Join(initramfsDir, "scripts", "local-top", "redwolf-storage")
	localTopData, err := os.ReadFile(localTopPath)
	if err != nil {
		t.Fatalf("failed reading local-top script %s: %v", localTopPath, err)
	}
	localStr := string(localTopData)
	if !strings.Contains(localStr, "mdadm --assemble") || !strings.Contains(localStr, "lvm vgchange -ay") {
		t.Errorf("local-top script missing assembly commands, got:\n%s", localStr)
	}
	fi2, _ := os.Stat(localTopPath)
	if fi2.Mode()&0111 == 0 {
		t.Errorf("expected local-top script to be executable (0755), got: %v", fi2.Mode())
	}
}

func TestEnsureLVMSymlinks(t *testing.T) {
	tempDir := t.TempDir()
	usrSbin := filepath.Join(tempDir, "usr", "sbin")
	_ = os.MkdirAll(usrSbin, 0755)

	// Create fake lvm binary
	lvmPath := filepath.Join(usrSbin, "lvm")
	_ = os.WriteFile(lvmPath, []byte("#!/bin/sh\necho LVM"), 0755)

	ensureLVMSymlinks(tempDir)

	for _, cmd := range []string{"vgchange", "vgscan", "vgs", "lvscan", "lvchange", "lvs", "pvscan", "pvs"} {
		linkPath := filepath.Join(usrSbin, cmd)
		target, err := os.Readlink(linkPath)
		if err != nil {
			t.Errorf("expected symlink for %s at %s, error: %v", cmd, linkPath, err)
			continue
		}
		if target != "lvm" {
			t.Errorf("expected symlink %s -> lvm, got -> %s", cmd, target)
		}
	}
}

func TestCopyExecutableFile(t *testing.T) {
	tempDir := t.TempDir()
	srcFile := filepath.Join(tempDir, "src-tool")
	dstFile := filepath.Join(tempDir, "nested", "dst-tool")

	content := "binary payload 123456"
	if err := os.WriteFile(srcFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed creating src file: %v", err)
	}

	if err := copyExecutableFile(srcFile, dstFile); err != nil {
		t.Fatalf("unexpected error copying executable: %v", err)
	}

	dstData, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("failed reading destination file: %v", err)
	}
	if string(dstData) != content {
		t.Errorf("content mismatch: got %s, expected %s", string(dstData), content)
	}

	fi, err := os.Stat(dstFile)
	if err != nil {
		t.Fatalf("failed statting dst file: %v", err)
	}
	if fi.Mode()&0111 == 0 {
		t.Errorf("expected dst file to have executable bit set (0755), got: %v", fi.Mode())
	}
}
