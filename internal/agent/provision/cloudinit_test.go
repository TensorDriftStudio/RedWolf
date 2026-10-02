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
	if !strings.Contains(userData, "root:TestPassword123!") {
		t.Fatalf("expected root password in chpasswd list")
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
