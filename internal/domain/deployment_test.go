package domain

import (
	"testing"
)

func TestDeploymentConfig_Validate(t *testing.T) {
	validBase := DeploymentConfig{
		NodeID:          "node-test-1",
		OS:              OSAlmaLinux9,
		TargetDrivePath: "/dev/disk/by-id/nvme-TEST_1234",
		RootPassword:    "SuperSecurePass123!",
		NetworkMode:     NetworkModeDHCP,
	}

	// 1. Valid configuration
	cfg := validBase
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}

	// 2. Slug normalization: "almalinux9" -> OSAlmaLinux9
	cfg = validBase
	cfg.OS = "almalinux9"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected slug 'almalinux9' to be valid, got: %v", err)
	}
	if cfg.OS != OSAlmaLinux9 {
		t.Fatalf("expected normalized OS '%s', got '%s'", OSAlmaLinux9, cfg.OS)
	}

	// 3. Slug normalization: "debian-12" -> OSDebian12
	cfg = validBase
	cfg.OS = "debian-12"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected slug 'debian-12' to be valid, got: %v", err)
	}
	if cfg.OS != OSDebian12 {
		t.Fatalf("expected normalized OS '%s', got '%s'", OSDebian12, cfg.OS)
	}

	// 4. Missing NodeID
	cfg = validBase
	cfg.NodeID = ""
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for empty NodeID, got nil")
	}

	// 5. Unsupported OS
	cfg = validBase
	cfg.OS = "Windows Server 2025"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for unsupported OS, got nil")
	}

	// 6. Missing TargetDrivePath
	cfg = validBase
	cfg.TargetDrivePath = "   "
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for empty TargetDrivePath, got nil")
	}

	// 7. Missing credentials
	cfg = validBase
	cfg.RootPassword = "short"
	cfg.SSHKeys = nil
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for insufficient credentials, got nil")
	}

	// 8. Static IP validation
	cfg = validBase
	cfg.NetworkMode = NetworkModeStatic
	cfg.StaticIP = "invalid-ip"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for invalid static IP, got nil")
	}

	cfg.StaticIP = "192.168.1.100"
	cfg.Gateway = "invalid-gw"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for invalid gateway, got nil")
	}

	cfg.Gateway = "192.168.1.1"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid static IP and gateway, got: %v", err)
	}

	// 9. Software RAID 1 validation
	cfg = validBase
	cfg.Storage = StorageConfig{
		RAIDLevel:    RAIDLevel1,
		TargetDrives: []string{"/dev/nvme0n1"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for RAID 1 with only 1 drive, got nil")
	}

	cfg.Storage.TargetDrives = []string{"/dev/nvme0n1", "/dev/nvme1n1"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid RAID 1 with 2 drives, got: %v", err)
	}

	// 10. Software RAID 10 validation
	cfg.Storage.RAIDLevel = RAIDLevel10
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for RAID 10 with 2 drives, got nil")
	}

	cfg.Storage.TargetDrives = []string{"/dev/sda", "/dev/sdb", "/dev/sdc", "/dev/sdd"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid RAID 10 with 4 drives, got: %v", err)
	}

	// 11. LVM validation
	cfg = validBase
	cfg.Storage = StorageConfig{
		LayoutMode: PartitioningLVM,
		LVMVolumes: []LVMVolumeConfig{
			{Name: "", MountPoint: "/", SizeGB: 50, FSType: "xfs"},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for empty LVM volume name, got nil")
	}

	cfg.Storage.LVMVolumes = []LVMVolumeConfig{
		{Name: "root", MountPoint: "/", SizeGB: 0, FSType: "xfs"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for LVM volume size < 1 GB, got nil")
	}

	cfg.Storage.LVMVolumes = []LVMVolumeConfig{
		{Name: "root", MountPoint: "/", SizeGB: 50, FSType: "xfs"},
		{Name: "var", MountPoint: "/var", SizeGB: 100, FSType: "xfs"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid LVM volumes, got: %v", err)
	}
}
