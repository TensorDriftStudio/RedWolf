package provision

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// InjectCloudInit writes NoCloud seed files directly to target rootfs by loop-mounting it in RAM.
func InjectCloudInit(ctx context.Context, targetDrivePath string, cfg domain.DeploymentConfig, bootMAC string) error {
	slog.InfoContext(ctx, "locating root partition for Cloud-Init NoCloud seed injection",
		"target_drive", targetDrivePath,
		"boot_mac", bootMAC,
	)

	// Step 1: Detect root partition
	rootPart, err := findRootPartition(ctx, targetDrivePath)
	if err != nil {
		return fmt.Errorf("failed detecting root partition on %s: %w", targetDrivePath, err)
	}

	mountPoint := "/mnt/redwolf-target"
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		return fmt.Errorf("failed creating mount point %s: %w", mountPoint, err)
	}

	// Step 2: Mount target rootfs
	slog.InfoContext(ctx, "mounting root partition", "partition", rootPart, "mountpoint", mountPoint)
	mountCmd := exec.CommandContext(ctx, "mount", rootPart, mountPoint)
	if out, err := mountCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed mounting partition %s to %s: %w (output: %s)", rootPart, mountPoint, err, string(out))
	}
	defer func() {
		_ = exec.Command("umount", mountPoint).Run()
	}()

	// Step 3: Create NoCloud seed directory
	seedDir := filepath.Join(mountPoint, "var", "lib", "cloud", "seed", "nocloud")
	if err := os.MkdirAll(seedDir, 0755); err != nil {
		return fmt.Errorf("failed creating seed directory %s: %w", seedDir, err)
	}

	// Step 4: Generate meta-data
	hostname := fmt.Sprintf("node-%s", strings.ToLower(strings.ReplaceAll(bootMAC, ":", "")))
	metaDataContent := fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", cfg.NodeID, hostname)
	if err := os.WriteFile(filepath.Join(seedDir, "meta-data"), []byte(metaDataContent), 0644); err != nil {
		return fmt.Errorf("failed writing meta-data: %w", err)
	}

	// Step 5: Generate user-data
	userDataContent := generateUserData(cfg)
	if err := os.WriteFile(filepath.Join(seedDir, "user-data"), []byte(userDataContent), 0644); err != nil {
		return fmt.Errorf("failed writing user-data: %w", err)
	}

	// Step 6: Generate network-config (Universal MAC matching)
	networkConfigContent := generateNetworkConfig(cfg, bootMAC)
	if err := os.WriteFile(filepath.Join(seedDir, "network-config"), []byte(networkConfigContent), 0644); err != nil {
		return fmt.Errorf("failed writing network-config: %w", err)
	}

	slog.InfoContext(ctx, "cloud-init nocloud seed injected successfully",
		"partition", rootPart,
		"seed_dir", seedDir,
		"boot_mac", bootMAC,
	)

	return nil
}

func generateUserData(cfg domain.DeploymentConfig) string {
	if strings.TrimSpace(cfg.CustomUserData) != "" {
		base := strings.TrimSpace(cfg.CustomUserData)
		if !strings.HasPrefix(base, "#cloud-config") {
			base = "#cloud-config\n" + base
		}
		var sb strings.Builder
		sb.WriteString(base)
		sb.WriteString("\n\n# RedWolf Injected Authentication & Hardening\n")
		sb.WriteString("users:\n")
		sb.WriteString("  - name: root\n")
		sb.WriteString("    lock_passwd: false\n")
		if len(cfg.SSHKeys) > 0 {
			sb.WriteString("    ssh_authorized_keys:\n")
			for _, key := range cfg.SSHKeys {
				if strings.TrimSpace(key) != "" {
					sb.WriteString(fmt.Sprintf("      - %s\n", strings.TrimSpace(key)))
				}
			}
		}
		if cfg.RootPassword != "" {
			sb.WriteString("\nchpasswd:\n")
			sb.WriteString("  list: |\n")
			sb.WriteString(fmt.Sprintf("    root:%s\n", cfg.RootPassword))
			sb.WriteString("  expire: false\n")
			sb.WriteString("ssh_pwauth: true\n")
		}
		return sb.String()
	}

	var sb strings.Builder
	sb.WriteString("#cloud-config\n")
	sb.WriteString("growpart:\n")
	sb.WriteString("  mode: auto\n")
	sb.WriteString("  devices: ['/']\n")
	sb.WriteString("  ignore_growpart_interface: false\n")
	sb.WriteString("resize_rootfs: true\n\n")

	sb.WriteString("users:\n")
	sb.WriteString("  - name: root\n")
	sb.WriteString("    lock_passwd: false\n")
	if len(cfg.SSHKeys) > 0 {
		sb.WriteString("    ssh_authorized_keys:\n")
		for _, key := range cfg.SSHKeys {
			if strings.TrimSpace(key) != "" {
				sb.WriteString(fmt.Sprintf("      - %s\n", strings.TrimSpace(key)))
			}
		}
	}

	if cfg.RootPassword != "" {
		sb.WriteString("\nchpasswd:\n")
		sb.WriteString("  list: |\n")
		sb.WriteString(fmt.Sprintf("    root:%s\n", cfg.RootPassword))
		sb.WriteString("  expire: false\n")
		sb.WriteString("ssh_pwauth: true\n")
	}

	sb.WriteString("\npackage_update: false\n")
	sb.WriteString("runcmd:\n")
	sb.WriteString("  - [ echo, 'RedWolf Bare-Metal Provisioning Complete' ]\n")

	return sb.String()
}

func generateNetworkConfig(cfg domain.DeploymentConfig, bootMAC string) string {
	var sb strings.Builder
	sb.WriteString("network:\n")
	sb.WriteString("  version: 2\n")
	sb.WriteString("  ethernets:\n")
	sb.WriteString("    id0:\n")
	sb.WriteString("      match:\n")
	sb.WriteString(fmt.Sprintf("        macaddress: \"%s\"\n", strings.ToLower(bootMAC)))
	sb.WriteString("      set-name: eth0\n")

	if cfg.NetworkMode == domain.NetworkModeStatic {
		sb.WriteString("      dhcp4: false\n")
		sb.WriteString("      dhcp6: false\n")
		cidr := cfg.NetmaskCIDR
		if cidr <= 0 || cidr > 32 {
			cidr = 24
		}
		sb.WriteString("      addresses:\n")
		sb.WriteString(fmt.Sprintf("        - %s/%d\n", cfg.StaticIP, cidr))

		if cfg.Gateway != "" {
			sb.WriteString("      routes:\n")
			sb.WriteString("        - to: default\n")
			sb.WriteString(fmt.Sprintf("          via: %s\n", cfg.Gateway))
		}

		if len(cfg.DNSServers) > 0 {
			sb.WriteString("      nameservers:\n")
			sb.WriteString("        addresses:\n")
			for _, dns := range cfg.DNSServers {
				sb.WriteString(fmt.Sprintf("          - %s\n", dns))
			}
		} else {
			sb.WriteString("      nameservers:\n")
			sb.WriteString("        addresses:\n")
			sb.WriteString("          - 1.1.1.1\n")
			sb.WriteString("          - 8.8.8.8\n")
		}
	} else {
		// DHCP mode
		sb.WriteString("      dhcp4: true\n")
		sb.WriteString("      dhcp6: false\n")
	}

	return sb.String()
}

type partInfo struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Size   uint64 `json:"size"`
	Type   string `json:"type"`
	FSType string `json:"fstype"`
	Label  string `json:"label"`
}

type partList struct {
	BlockDevices []partInfo `json:"blockdevices"`
}

func findRootPartition(ctx context.Context, targetDrivePath string) (string, error) {
	cmd := exec.CommandContext(ctx, "lsblk", "-J", "-b", "-o", "NAME,PATH,SIZE,TYPE,FSTYPE,LABEL", targetDrivePath)
	out, err := cmd.Output()
	if err != nil {
		// Fallback to convention: nvme0n1p3 or sda3
		if strings.Contains(targetDrivePath, "nvme") {
			return targetDrivePath + "p3", nil
		}
		return targetDrivePath + "3", nil
	}

	var data partList
	if err := json.Unmarshal(out, &data); err != nil {
		if strings.Contains(targetDrivePath, "nvme") {
			return targetDrivePath + "p3", nil
		}
		return targetDrivePath + "3", nil
	}

	// Look for partition with label root, or largest xfs/ext4 partition
	var candidate string
	var largestSize uint64

	for _, p := range data.BlockDevices {
		if p.Type != "part" {
			continue
		}
		labelLower := strings.ToLower(p.Label)
		if strings.Contains(labelLower, "root") {
			return p.Path, nil
		}
		if (p.FSType == "xfs" || p.FSType == "ext4") && p.Size > largestSize {
			largestSize = p.Size
			candidate = p.Path
		}
	}

	if candidate != "" {
		return candidate, nil
	}

	// Default fallback
	if strings.Contains(targetDrivePath, "nvme") {
		return targetDrivePath + "p3", nil
	}
	return targetDrivePath + "3", nil
}
