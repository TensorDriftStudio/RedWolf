package provision

import (
	"context"
	"testing"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

func TestResolvePartitionPath(t *testing.T) {
	cases := []struct {
		diskPath string
		partNum  int
		expected string
	}{
		{"/dev/sda", 1, "/dev/sda1"},
		{"/dev/sda", 2, "/dev/sda2"},
		{"/dev/sdb", 1, "/dev/sdb1"},
		{"/dev/nvme0n1", 1, "/dev/nvme0n1p1"},
		{"/dev/nvme0n1", 2, "/dev/nvme0n1p2"},
		{"/dev/nvme1n1", 1, "/dev/nvme1n1p1"},
		{"/dev/loop0", 1, "/dev/loop0p1"},
		{"/dev/vda", 1, "/dev/vda1"},
	}

	for _, c := range cases {
		result := resolvePartitionPath(c.diskPath, c.partNum)
		if result != c.expected {
			t.Errorf("resolvePartitionPath(%q, %d) = %q, expected %q", c.diskPath, c.partNum, result, c.expected)
		}
	}
}

func TestSetupStorageArchitecture_Standard(t *testing.T) {
	ctx := context.Background()
	cfg := domain.DeploymentConfig{
		TargetDrivePath: "/dev/sda",
		Storage: domain.StorageConfig{
			LayoutMode: domain.PartitioningStandard,
		},
	}

	res, err := SetupStorageArchitecture(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.TargetDrive != "/dev/sda" {
		t.Errorf("expected TargetDrive '/dev/sda', got %q", res.TargetDrive)
	}
	if res.IsSoftwareRAID {
		t.Errorf("expected IsSoftwareRAID false, got true")
	}
	if len(res.ESPDrives) != 1 || res.ESPDrives[0] != "/dev/sda" {
		t.Errorf("expected ESPDrives ['/dev/sda'], got %v", res.ESPDrives)
	}
}

func TestInjectMDADMConfig_Paths(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()

	// Call InjectMDADMConfig (mdadm might not have arrays in test environment, but directories and files are exercised)
	err := InjectMDADMConfig(ctx, tempDir)
	if err != nil {
		t.Fatalf("unexpected error injecting mdadm config: %v", err)
	}
}

func TestResolveLVMDeviceNode(t *testing.T) {
	node := resolveLVMDeviceNode("vg_system", "root")
	if node != "/dev/vg_system/root" && node != "/dev/mapper/vg_system-root" {
		t.Errorf("unexpected resolved LVM device node: %q", node)
	}

	hyphenNode := resolveLVMDeviceNode("vg-test", "lv-data")
	if hyphenNode != "/dev/vg-test/lv-data" && hyphenNode != "/dev/mapper/vg--test-lv--data" {
		t.Errorf("unexpected resolved hyphenated LVM device node: %q", hyphenNode)
	}
}

