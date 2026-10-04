package provision

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

func TestGenerateUserData(t *testing.T) {
	cfg := domain.DeploymentConfig{
		NodeID:       "node-test-1",
		OS:           domain.OSAlmaLinux9,
		RootPassword: "TestPassword123!",
		SSHKeys: []string{
			"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC test@enterprise",
		},
	}

	userData := generateUserData(cfg)

	if !strings.Contains(userData, "#cloud-config") {
		t.Fatalf("expected #cloud-config header, got: %s", userData)
	}
	if !strings.Contains(userData, "growpart:") {
		t.Fatalf("expected growpart stanza in user-data")
	}
	if !strings.Contains(userData, "resize_rootfs: true") {
		t.Fatalf("expected resize_rootfs stanza in user-data")
	}
	if !strings.Contains(userData, "ssh-rsa AAAAB3NzaC1yc2E") {
		t.Fatalf("expected SSH public key in user-data")
	}
	if !strings.Contains(userData, "disable_root: false") {
		t.Fatalf("expected disable_root: false in user-data")
	}
	if !strings.Contains(userData, "ssh_pwauth: true") {
		t.Fatalf("expected ssh_pwauth: true in user-data")
	}
	if !strings.Contains(userData, "PermitRootLogin yes") {
		t.Fatalf("expected PermitRootLogin yes in write_files")
	}
	if !strings.Contains(userData, `passwd: "$6$`) {
		t.Fatalf("expected passwd hash under root user in user-data")
	}
	if !strings.Contains(userData, "root:$6$") {
		t.Fatalf("expected SHA-512 crypt root password in chpasswd list, got: %s", userData)
	}
	if strings.Contains(userData, "TestPassword123!") {
		t.Fatalf("plaintext password must never appear in user-data!")
	}
}

func TestGenerateNetworkConfig_DHCP(t *testing.T) {
	cfg := domain.DeploymentConfig{
		NodeID:      "node-test-1",
		NetworkMode: domain.NetworkModeDHCP,
	}
	bootMAC := "52:54:00:12:34:56"

	netConfig := generateNetworkConfig(cfg, bootMAC)

	if !strings.Contains(netConfig, "version: 2") {
		t.Fatalf("expected Netplan version 2")
	}
	if !strings.Contains(netConfig, `macaddress: "52:54:00:12:34:56"`) {
		t.Fatalf("expected MAC address matching for %s, got: %s", bootMAC, netConfig)
	}
	if !strings.Contains(netConfig, "dhcp4: true") {
		t.Fatalf("expected dhcp4: true in dhcp mode")
	}
}

func TestGenerateNetworkConfig_Static(t *testing.T) {
	cfg := domain.DeploymentConfig{
		NodeID:      "node-test-2",
		NetworkMode: domain.NetworkModeStatic,
		StaticIP:    "10.10.100.50",
		NetmaskCIDR: 24,
		Gateway:     "10.10.100.1",
		DNSServers:  []string{"1.1.1.1", "9.9.9.9"},
	}
	bootMAC := "AC:1F:6B:80:99:AA"

	netConfig := generateNetworkConfig(cfg, bootMAC)

	if !strings.Contains(netConfig, "version: 2") {
		t.Fatalf("expected Netplan version 2")
	}
	if !strings.Contains(netConfig, `macaddress: "ac:1f:6b:80:99:aa"`) {
		t.Fatalf("expected lowercase MAC match, got: %s", netConfig)
	}
	if !strings.Contains(netConfig, "10.10.100.50/24") {
		t.Fatalf("expected CIDR IP address format, got: %s", netConfig)
	}
	if !strings.Contains(netConfig, "via: 10.10.100.1") {
		t.Fatalf("expected default gateway route, got: %s", netConfig)
	}
	if !strings.Contains(netConfig, "- 1.1.1.1") || !strings.Contains(netConfig, "- 9.9.9.9") {
		t.Fatalf("expected custom DNS servers, got: %s", netConfig)
	}
}

func TestFindRootPartitionFromJSON_NestedChildren(t *testing.T) {
	// Sample lsblk JSON simulating AlmaLinux 9 cloud raw image on NVMe
	almaJSON := []byte(`{
		"blockdevices": [
			{
				"name": "nvme0n1",
				"path": "/dev/nvme0n1",
				"size": 53687091200,
				"type": "disk",
				"children": [
					{
						"name": "nvme0n1p1",
						"path": "/dev/nvme0n1p1",
						"size": 1048576,
						"type": "part",
						"fstype": null,
						"label": null
					},
					{
						"name": "nvme0n1p2",
						"path": "/dev/nvme0n1p2",
						"size": 209715200,
						"type": "part",
						"fstype": "vfat",
						"label": "EFI"
					},
					{
						"name": "nvme0n1p3",
						"path": "/dev/nvme0n1p3",
						"size": 53476327424,
						"type": "part",
						"fstype": "xfs",
						"label": "root"
					}
				]
			}
		]
	}`)

	part, err := findRootPartitionFromJSON(almaJSON, "/dev/nvme0n1")
	if err != nil {
		t.Fatalf("unexpected error finding root partition: %v", err)
	}
	if part != "/dev/nvme0n1p3" {
		t.Fatalf("expected /dev/nvme0n1p3, got %s", part)
	}

	// Sample lsblk JSON simulating Debian 12 cloud raw image on SAS/SATA (root is partition 1)
	debianJSON := []byte(`{
		"blockdevices": [
			{
				"name": "sda",
				"path": "/dev/sda",
				"size": 53687091200,
				"type": "disk",
				"children": [
					{
						"name": "sda1",
						"path": "/dev/sda1",
						"size": 53150220288,
						"type": "part",
						"fstype": "ext4",
						"label": "rootfs"
					},
					{
						"name": "sda15",
						"path": "/dev/sda15",
						"size": 130023424,
						"type": "part",
						"fstype": "vfat",
						"label": "ESP"
					}
				]
			}
		]
	}`)

	partDebian, err := findRootPartitionFromJSON(debianJSON, "/dev/sda")
	if err != nil {
		t.Fatalf("unexpected error finding Debian root partition: %v", err)
	}
	if partDebian != "/dev/sda1" {
		t.Fatalf("expected /dev/sda1, got %s", partDebian)
	}
}

func TestFindRootPartition_PartLabelRoot(t *testing.T) {
	// Simulating AlmaLinux 9/10 partition 4 where filesystem label is null but GPT PARTLABEL is "root"
	almaJSON := []byte(`{
		"blockdevices": [
			{
				"name": "nvme0n1",
				"path": "/dev/nvme0n1",
				"size": 22548578304,
				"type": "disk",
				"children": [
					{
						"name": "nvme0n1p1",
						"path": "/dev/nvme0n1p1",
						"size": 1048576,
						"type": "part",
						"partlabel": "biosboot"
					},
					{
						"name": "nvme0n1p2",
						"path": "/dev/nvme0n1p2",
						"size": 209715200,
						"type": "part",
						"partlabel": "EFI System Partition",
						"fstype": "vfat"
					},
					{
						"name": "nvme0n1p3",
						"path": "/dev/nvme0n1p3",
						"size": 1073741824,
						"type": "part",
						"partlabel": "boot",
						"fstype": "xfs"
					},
					{
						"name": "nvme0n1p4",
						"path": "/dev/nvme0n1p4",
						"size": 9448882176,
						"type": "part",
						"partlabel": "root",
						"fstype": "xfs",
						"label": null
					}
				]
			}
		]
	}`)

	part, err := findRootPartitionFromJSON(almaJSON, "/dev/nvme0n1")
	if err != nil {
		t.Fatalf("failed finding root partition: %v", err)
	}
	if part != "/dev/nvme0n1p4" {
		t.Errorf("expected /dev/nvme0n1p4 from PARTLABEL='root', got %s", part)
	}
}

func TestFindRootPartition_AlmaLinux10_Standard(t *testing.T) {
	// Official AlmaLinux 10 GenericCloud NVMe layout
	alma10JSON := []byte(`{
		"blockdevices": [
			{
				"name": "nvme0n1",
				"path": "/dev/nvme0n1",
				"size": 53687091200,
				"type": "disk",
				"children": [
					{
						"name": "nvme0n1p1",
						"path": "/dev/nvme0n1p1",
						"size": 1048576,
						"type": "part",
						"partlabel": "biosboot"
					},
					{
						"name": "nvme0n1p2",
						"path": "/dev/nvme0n1p2",
						"size": 209715200,
						"type": "part",
						"partlabel": "EFI System Partition",
						"fstype": "vfat"
					},
					{
						"name": "nvme0n1p3",
						"path": "/dev/nvme0n1p3",
						"size": 1073741824,
						"type": "part",
						"partlabel": "boot",
						"fstype": "xfs"
					},
					{
						"name": "nvme0n1p4",
						"path": "/dev/nvme0n1p4",
						"size": 9448882176,
						"type": "part",
						"partlabel": "root",
						"fstype": "xfs"
					}
				]
			}
		]
	}`)

	part, err := findRootPartitionFromJSON(alma10JSON, "/dev/nvme0n1", domain.OSAlmaLinux10)
	if err != nil {
		t.Fatalf("failed finding AlmaLinux 10 root partition: %v", err)
	}
	if part != "/dev/nvme0n1p4" {
		t.Errorf("expected /dev/nvme0n1p4 for AlmaLinux 10, got %s", part)
	}
}

func TestFindRootPartition_AlmaLinux10_UEFIOnly_3Partitions(t *testing.T) {
	// UEFI-only AlmaLinux 10 layout (no biosboot: p1=ESP, p2=boot, p3=root)
	alma10UEFIJSON := []byte(`{
		"blockdevices": [
			{
				"name": "nvme0n1",
				"path": "/dev/nvme0n1",
				"size": 53687091200,
				"type": "disk",
				"children": [
					{
						"name": "nvme0n1p1",
						"path": "/dev/nvme0n1p1",
						"size": 629145600,
						"type": "part",
						"partlabel": "EFI System Partition",
						"fstype": "vfat"
					},
					{
						"name": "nvme0n1p2",
						"path": "/dev/nvme0n1p2",
						"size": 1073741824,
						"type": "part",
						"partlabel": "boot",
						"fstype": "xfs"
					},
					{
						"name": "nvme0n1p3",
						"path": "/dev/nvme0n1p3",
						"size": 9448882176,
						"type": "part",
						"partlabel": "root",
						"fstype": "xfs"
					}
				]
			}
		]
	}`)

	part, err := findRootPartitionFromJSON(alma10UEFIJSON, "/dev/nvme0n1", domain.OSAlmaLinux10)
	if err != nil {
		t.Fatalf("failed finding AlmaLinux 10 3-partition root: %v", err)
	}
	if part != "/dev/nvme0n1p3" {
		t.Errorf("expected /dev/nvme0n1p3 for AlmaLinux 10 3-partition layout, got %s", part)
	}
}

func TestFindRootPartition_Debian13_NVMe(t *testing.T) {
	// Official Debian 13 GenericCloud on NVMe (partitions 1, 14, 15, no "root" in labels)
	debian13JSON := []byte(`{
		"blockdevices": [
			{
				"name": "nvme0n1",
				"path": "/dev/nvme0n1",
				"size": 53687091200,
				"type": "disk",
				"children": [
					{
						"name": "nvme0n1p1",
						"path": "/dev/nvme0n1p1",
						"size": 2048000000,
						"type": "part",
						"fstype": "ext4",
						"label": ""
					},
					{
						"name": "nvme0n1p14",
						"path": "/dev/nvme0n1p14",
						"size": 4194304,
						"type": "part",
						"fstype": null,
						"label": ""
					},
					{
						"name": "nvme0n1p15",
						"path": "/dev/nvme0n1p15",
						"size": 130023424,
						"type": "part",
						"fstype": "vfat",
						"label": "ESP"
					}
				]
			}
		]
	}`)

	part, err := findRootPartitionFromJSON(debian13JSON, "/dev/nvme0n1", domain.OSDebian13)
	if err != nil {
		t.Fatalf("failed finding Debian 13 root partition: %v", err)
	}
	if part != "/dev/nvme0n1p1" {
		t.Errorf("expected /dev/nvme0n1p1 for Debian 13, got %s", part)
	}
}

func TestFindRootPartition_Debian13_ResistantToStalePartition4(t *testing.T) {
	// Disk with a stale partition 4 from prior AlmaLinux install (40GB)
	// along with Debian 13 partitions (1, 14, 15).
	// With domain.OSDebian13, it must select /dev/nvme0n1p1 and reject stale /dev/nvme0n1p4!
	staleJSON := []byte(`{
		"blockdevices": [
			{
				"name": "nvme0n1",
				"path": "/dev/nvme0n1",
				"size": 53687091200,
				"type": "disk",
				"children": [
					{
						"name": "nvme0n1p1",
						"path": "/dev/nvme0n1p1",
						"size": 2048000000,
						"type": "part",
						"fstype": "ext4",
						"label": ""
					},
					{
						"name": "nvme0n1p4",
						"path": "/dev/nvme0n1p4",
						"size": 42949672960,
						"type": "part",
						"fstype": null,
						"label": null
					},
					{
						"name": "nvme0n1p14",
						"path": "/dev/nvme0n1p14",
						"size": 4194304,
						"type": "part",
						"fstype": null,
						"label": ""
					},
					{
						"name": "nvme0n1p15",
						"path": "/dev/nvme0n1p15",
						"size": 130023424,
						"type": "part",
						"fstype": "vfat",
						"label": "ESP"
					}
				]
			}
		]
	}`)

	part, err := findRootPartitionFromJSON(staleJSON, "/dev/nvme0n1", domain.OSDebian13)
	if err != nil {
		t.Fatalf("failed finding Debian 13 root partition with stale p4: %v", err)
	}
	if part != "/dev/nvme0n1p1" {
		t.Fatalf("expected /dev/nvme0n1p1 for Debian 13 despite larger stale partition 4, got %s", part)
	}

	// Conversely, for AlmaLinux 9, it should select partition 4
	partAlma, err := findRootPartitionFromJSON(staleJSON, "/dev/nvme0n1", domain.OSAlmaLinux9)
	if err != nil {
		t.Fatalf("failed finding AlmaLinux root partition: %v", err)
	}
	if partAlma != "/dev/nvme0n1p4" {
		t.Fatalf("expected /dev/nvme0n1p4 for AlmaLinux 9, got %s", partAlma)
	}
}

func TestWriteNoCloudSeeds(t *testing.T) {
	tempDir := t.TempDir()
	cfg := domain.DeploymentConfig{
		NodeID:       "node-12345",
		OS:           domain.OSAlmaLinux9,
		RootPassword: "StrongPassword2026#",
		NetworkMode:  domain.NetworkModeDHCP,
	}
	bootMAC := "52:54:00:11:22:33"

	err := WriteNoCloudSeeds(tempDir, cfg, bootMAC)
	if err != nil {
		t.Fatalf("unexpected error writing seeds: %v", err)
	}

	seedDir := filepath.Join(tempDir, "var", "lib", "cloud", "seed", "nocloud")
	meta, err := os.ReadFile(filepath.Join(seedDir, "meta-data"))
	if err != nil {
		t.Fatalf("failed reading meta-data: %v", err)
	}
	if !strings.Contains(string(meta), "node-12345") {
		t.Errorf("expected instance-id in meta-data, got %s", string(meta))
	}

	user, err := os.ReadFile(filepath.Join(seedDir, "user-data"))
	if err != nil {
		t.Fatalf("failed reading user-data: %v", err)
	}
	if !strings.Contains(string(user), "#cloud-config") {
		t.Errorf("expected #cloud-config header, got %s", string(user))
	}

	net, err := os.ReadFile(filepath.Join(seedDir, "network-config"))
	if err != nil {
		t.Fatalf("failed reading network-config: %v", err)
	}
	if !strings.Contains(string(net), "52:54:00:11:22:33") {
		t.Errorf("expected macaddress in network-config, got %s", string(net))
	}
}

func TestDirectInjectSecurityCredentials(t *testing.T) {
	tempDir := t.TempDir()

	// Create fake existing /etc/shadow with locked root
	etcDir := filepath.Join(tempDir, "etc")
	if err := os.MkdirAll(etcDir, 0755); err != nil {
		t.Fatalf("failed creating test etc dir: %v", err)
	}
	initialShadow := "root:*:19800:0:99999:7:::\nbin:*:19800:0:99999:7:::\n"
	if err := os.WriteFile(filepath.Join(etcDir, "shadow"), []byte(initialShadow), 0600); err != nil {
		t.Fatalf("failed writing initial shadow: %v", err)
	}

	cfg := domain.DeploymentConfig{
		NodeID:       "node-sec-test",
		RootPassword: "SuperSecurePass2026!",
		SSHKeys: []string{
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG... admin@redwolf",
		},
	}

	if err := DirectInjectSecurityCredentials(tempDir, cfg); err != nil {
		t.Fatalf("unexpected error injecting security credentials: %v", err)
	}

	// 1. Verify shadow was updated with valid SHA-512 hash
	updatedShadow, err := os.ReadFile(filepath.Join(etcDir, "shadow"))
	if err != nil {
		t.Fatalf("failed reading updated shadow: %v", err)
	}
	lines := strings.Split(string(updatedShadow), "\n")
	rootFound := false
	for _, l := range lines {
		if strings.HasPrefix(l, "root:") {
			rootFound = true
			parts := strings.Split(l, ":")
			if !strings.HasPrefix(parts[1], "$6$") {
				t.Fatalf("expected $6$ password hash in /etc/shadow for root, got: %s", parts[1])
			}
		}
	}
	if !rootFound {
		t.Fatalf("root entry missing from updated /etc/shadow")
	}

	// 2. Verify OpenSSH drop-in config was created
	dropinPath := filepath.Join(etcDir, "ssh", "sshd_config.d", "99-redwolf-root.conf")
	dropinData, err := os.ReadFile(dropinPath)
	if err != nil {
		t.Fatalf("failed reading sshd drop-in config: %v", err)
	}
	if !strings.Contains(string(dropinData), "PermitRootLogin yes") {
		t.Errorf("expected PermitRootLogin yes in sshd dropin")
	}
	if !strings.Contains(string(dropinData), "PasswordAuthentication yes") {
		t.Errorf("expected PasswordAuthentication yes in sshd dropin")
	}

	// 3. Verify authorized_keys was written
	authKeysPath := filepath.Join(tempDir, "root", ".ssh", "authorized_keys")
	keysData, err := os.ReadFile(authKeysPath)
	if err != nil {
		t.Fatalf("failed reading root authorized_keys: %v", err)
	}
	if !strings.Contains(string(keysData), "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5") {
		t.Errorf("expected SSH public key in authorized_keys")
	}
}

