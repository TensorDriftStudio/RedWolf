package provision

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestGenerateLVMFstab_Ext4(t *testing.T) {
	tempDir := t.TempDir()
	layout := &StorageLayoutResult{
		TargetDrive:    "/dev/vg_system/root",
		RootFSType:     "ext4",
		BootPartition:  "/dev/nvme0n1p2",
		BootFSType:     "ext4",
		ESPPartition:   "/dev/nvme0n1p1",
		SwapDevice:     "/dev/vg_system/swap",
		LVMVolumes: []domain.LVMVolumeConfig{
			{Name: "root", MountPoint: "/", SizeGB: 10, FSType: "ext4"},
		},
	}

	ctx := context.Background()
	err := generateLVMFstab(ctx, tempDir, layout, nil)
	if err != nil {
		t.Fatalf("unexpected error generating LVM fstab: %v", err)
	}

	fstabContent, err := os.ReadFile(filepath.Join(tempDir, "etc", "fstab"))
	if err != nil {
		t.Fatalf("failed reading generated fstab: %v", err)
	}

	str := string(fstabContent)
	if !strings.Contains(str, "/dev/mapper/vg_system-root / ext4 defaults") {
		t.Errorf("expected root LV entry with ext4 in fstab, got:\n%s", str)
	}
	if !strings.Contains(str, "/boot ext4 defaults") {
		t.Errorf("expected boot entry with ext4 in fstab, got:\n%s", str)
	}
}

func TestSelectTempImagePath(t *testing.T) {
	tempDir := t.TempDir()
	path := selectTempImagePath(tempDir)
	if path == "" {
		t.Fatalf("expected non-empty temp image path")
	}
	// Path should be either /tmp/... or under tempDir
	if path != "/tmp/redwolf-cloud-image.raw" && !strings.HasPrefix(path, tempDir) {
		t.Errorf("unexpected temp image path: %s", path)
	}
}

func TestStreamImage_RegularFile(t *testing.T) {
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "subdir", "test-image.raw")

	sampleData := []byte("RedWolf Provisioning Test Stream Content 1234567890")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(sampleData)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(sampleData)
	}))
	defer server.Close()

	ctx := context.Background()
	err := StreamImage(ctx, server.URL, targetPath, nil)
	if err != nil {
		t.Fatalf("unexpected error streaming to regular file: %v", err)
	}

	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed reading created image file: %v", err)
	}

	if string(content) != string(sampleData) {
		t.Errorf("content mismatch: got %q, expected %q", string(content), string(sampleData))
	}
}

