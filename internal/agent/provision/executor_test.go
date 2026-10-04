package provision

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

func TestGenerateLVMFstab(t *testing.T) {
	tempDir := t.TempDir()
	layout := &StorageLayoutResult{
		TargetDrive:    "/dev/vg_system/root",
		BootPartition:  "/dev/nvme0n1p2",
		ESPPartition:   "/dev/nvme0n1p1",
		SwapDevice:     "/dev/vg_system/swap",
		LVMVolumes: []domain.LVMVolumeConfig{
			{Name: "root", MountPoint: "/", SizeGB: 10, FSType: "xfs"},
			{Name: "var", MountPoint: "/var", SizeGB: 5, FSType: "xfs"},
			{Name: "home", MountPoint: "/home", SizeGB: 2, FSType: "xfs"},
		},
	}

	subMounts := []lvmMountEntry{
		{device: "/dev/vg_system/var", mountPoint: "/var", fsType: "xfs"},
		{device: "/dev/vg_system/home", mountPoint: "/home", fsType: "xfs"},
	}

	ctx := context.Background()
	err := generateLVMFstab(ctx, tempDir, layout, subMounts)
	if err != nil {
		t.Fatalf("unexpected error generating LVM fstab: %v", err)
	}

	fstabContent, err := os.ReadFile(filepath.Join(tempDir, "etc", "fstab"))
	if err != nil {
		t.Fatalf("failed reading generated fstab: %v", err)
	}

	str := string(fstabContent)
	if !strings.Contains(str, "/dev/mapper/vg_system-root / xfs defaults") {
		t.Errorf("expected root LV entry in fstab, got:\n%s", str)
	}
	if !strings.Contains(str, "/dev/mapper/vg_system-var /var xfs defaults") {
		t.Errorf("expected var LV entry in fstab, got:\n%s", str)
	}
	if !strings.Contains(str, "/dev/mapper/vg_system-home /home xfs defaults") {
		t.Errorf("expected home LV entry in fstab, got:\n%s", str)
	}
	if !strings.Contains(str, "/dev/mapper/vg_system-swap none swap sw") {
		t.Errorf("expected swap entry in fstab, got:\n%s", str)
	}
	if !strings.Contains(str, "/boot") {
		t.Errorf("expected /boot entry in fstab, got:\n%s", str)
	}
	if !strings.Contains(str, "/boot/efi vfat") {
		t.Errorf("expected /boot/efi entry in fstab, got:\n%s", str)
	}
}
