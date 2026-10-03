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
