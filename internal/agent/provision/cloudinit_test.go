package provision

import (
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
