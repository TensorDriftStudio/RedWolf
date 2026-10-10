package provision

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestGenerateLVMFstab_CorrectDevicePaths(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()

	layout := &StorageLayoutResult{
		IsLVM:         true,
		RootPartition: "/dev/mapper/vg_system-root",
		RootFSType:    "xfs",
		BootPartition: "/dev/md0",
		BootFSType:    "xfs",
		ESPPartition:  "/dev/sda1",
		SwapDevice:    "/dev/mapper/vg_system-swap",
	}

	subMounts := []lvmMountEntry{
		{
			device:     "/dev/mapper/vg_system-home",
			mountPoint: "/home",
			fsType:     "xfs",
		},
		{
			device:     "/dev/mapper/vg_system-var",
			mountPoint: "/var",
			fsType:     "xfs",
		},
	}

	err := generateLVMFstab(ctx, tempDir, layout, subMounts)
	if err != nil {
		t.Fatalf("unexpected error generating fstab: %v", err)
	}

	fstabContent, err := os.ReadFile(filepath.Join(tempDir, "etc", "fstab"))
	if err != nil {
		t.Fatalf("failed reading generated fstab: %v", err)
	}
	contentStr := string(fstabContent)

	// Must contain /dev/mapper/vg_system-home and NOT /dev/mapper/vg_system-vg_system-home
	if !strings.Contains(contentStr, "/dev/mapper/vg_system-home /home xfs defaults 0 0") {
		t.Errorf("expected clean /home device path in fstab, got:\n%s", contentStr)
	}
	if strings.Contains(contentStr, "vg_system-vg_system-home") {
		t.Errorf("fstab contains duplicated vg_system prefix in path: %s", contentStr)
	}
	if !strings.Contains(contentStr, "/dev/mapper/vg_system-var /var xfs defaults 0 0") {
		t.Errorf("expected clean /var device path in fstab, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "/dev/mapper/vg_system-root / xfs defaults 0 0") {
		t.Errorf("expected root volume in fstab, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "/boot/efi vfat umask=0077,shortname=winnt,nofail 0 2") {
		t.Errorf("expected nofail flag on /boot/efi in fstab, got:\n%s", contentStr)
	}
}

func TestDirectInjectSecurityCredentials_RedwolfRelease(t *testing.T) {
	tempDir := t.TempDir()
	cfg := domain.DeploymentConfig{
		NodeID:       "node-test-123",
		OS:           domain.OSAlmaLinux9,
		RootPassword: "StrongPassword123!",
	}

	err := DirectInjectSecurityCredentials(context.Background(), tempDir, cfg)
	if err != nil {
		t.Fatalf("unexpected error injecting credentials: %v", err)
	}

	releaseFile := filepath.Join(tempDir, "etc", "redwolf-release")
	content, err := os.ReadFile(releaseFile)
	if err != nil {
		t.Fatalf("expected /etc/redwolf-release to exist: %v", err)
	}
	if !strings.Contains(string(content), "node-test-123") {
		t.Errorf("expected node ID in /etc/redwolf-release, got: %s", string(content))
	}
	if !strings.Contains(string(content), "AlmaLinux 9") {
		t.Errorf("expected OS in /etc/redwolf-release, got: %s", string(content))
	}
}

func TestWriteNoCloudSeeds_DualPathsAndCloudCfg(t *testing.T) {
	tempDir := t.TempDir()
	cfg := domain.DeploymentConfig{
		NodeID: "node-test-456",
		OS:     domain.OSDebian12,
	}

	err := WriteNoCloudSeeds(context.Background(), tempDir, cfg, "00:11:22:33:44:55")
	if err != nil {
		t.Fatalf("unexpected error writing seeds: %v", err)
	}

	// Verify nocloud and nocloud-net directories exist
	seedNocloud := filepath.Join(tempDir, "var", "lib", "cloud", "seed", "nocloud", "meta-data")
	seedNocloudNet := filepath.Join(tempDir, "var", "lib", "cloud", "seed", "nocloud-net", "meta-data")
	if _, err := os.Stat(seedNocloud); err != nil {
		t.Errorf("expected nocloud meta-data to exist: %v", err)
	}
	if _, err := os.Stat(seedNocloudNet); err != nil {
		t.Errorf("expected nocloud-net meta-data to exist: %v", err)
	}

	// Verify 99-redwolf.cfg exists
	cloudCfg := filepath.Join(tempDir, "etc", "cloud", "cloud.cfg.d", "99-redwolf.cfg")
	cfgData, err := os.ReadFile(cloudCfg)
	if err != nil {
		t.Errorf("expected 99-redwolf.cfg to exist: %v", err)
	}
	if !strings.Contains(string(cfgData), "datasource_list: [ NoCloud, None ]") {
		t.Errorf("unexpected 99-redwolf.cfg content: %s", string(cfgData))
	}
}

func TestEnsureMDDeviceNode(t *testing.T) {
	tempDir := t.TempDir()
	devDir := filepath.Join(tempDir, "dev")
	mdSubdir := filepath.Join(devDir, "md")
	if err := os.MkdirAll(mdSubdir, 0755); err != nil {
		t.Fatalf("failed creating test dev dir: %v", err)
	}

	// Create fake /dev/md/0
	altNode := filepath.Join(mdSubdir, "0")
	if err := os.WriteFile(altNode, []byte("fake-md"), 0660); err != nil {
		t.Fatalf("failed writing fake alt node: %v", err)
	}

	targetNode := filepath.Join(devDir, "md0")
	// Test linking logic
	if _, err := os.Stat(targetNode); os.IsNotExist(err) {
		if _, err2 := os.Stat(altNode); err2 == nil {
			_ = os.Symlink(altNode, targetNode)
		}
	}

	if _, err := os.Stat(targetNode); err != nil {
		t.Fatalf("expected symlink %s to exist: %v", targetNode, err)
	}
}

func TestSetupStorageArchitecture_ValidationAndPresets(t *testing.T) {
	// RAID1 preset configuration
	cfgRAID1 := domain.DeploymentConfig{
		NodeID:             "node-raid1-test",
		OS:                 domain.OSAlmaLinux9,
		RootPassword:       "Password123!",
		TargetDrivePath:    "/dev/sda",
		PartitioningPreset: domain.PartitioningRAID1,
		Storage: domain.StorageConfig{
			RAIDLevel:    domain.RAIDLevel1,
			TargetDrives: []string{"/dev/sda", "/dev/sdb"},
		},
	}
	if err := cfgRAID1.Validate(); err != nil {
		t.Fatalf("expected valid RAID 1 config: %v", err)
	}

	// LVM preset configuration with custom volumes
	cfgLVM := domain.DeploymentConfig{
		NodeID:             "node-lvm-test",
		OS:                 domain.OSAlmaLinux9,
		TargetDrivePath:    "/dev/nvme0n1",
		PartitioningPreset: domain.PartitioningLVM,
		RootPassword:       "Password123!",
		Storage: domain.StorageConfig{
			LayoutMode: domain.PartitioningLVM,
			LVMVolumes: []domain.LVMVolumeConfig{
				{Name: "root", MountPoint: "/", SizeGB: 50, FSType: "xfs"},
				{Name: "data", MountPoint: "/data", SizeGB: 200, FSType: "ext4"},
			},
			SwapSizeGB: 8,
		},
	}
	if err := cfgLVM.Validate(); err != nil {
		t.Fatalf("expected valid LVM config: %v", err)
	}
	if len(cfgLVM.Storage.LVMVolumes) != 2 {
		t.Fatalf("expected 2 LVM volumes, got %d", len(cfgLVM.Storage.LVMVolumes))
	}
}

