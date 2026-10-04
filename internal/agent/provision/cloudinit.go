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
	"time"

	"github.com/tensordriftstudio/redwolf/internal/adapter/crypto"
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
		umountCtx, umountCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer umountCancel()
		_ = exec.CommandContext(umountCtx, "umount", mountPoint).Run()
	}()

	// Step 2.1: Online expand root filesystem to use the full partition capacity
	slog.InfoContext(ctx, "auto-expanding root filesystem to full partition capacity", "partition", rootPart, "mountpoint", mountPoint)
	xfsGrowCmd := exec.CommandContext(ctx, "xfs_growfs", mountPoint)
	if out, err := xfsGrowCmd.CombinedOutput(); err == nil {
		slog.InfoContext(ctx, "xfs root filesystem expanded successfully", "partition", rootPart, "output", strings.TrimSpace(string(out)))
	} else {
		// If not XFS, attempt online ext4 resize
		resizeCmd := exec.CommandContext(ctx, "resize2fs", rootPart)
		if rOut, rErr := resizeCmd.CombinedOutput(); rErr == nil {
			slog.InfoContext(ctx, "ext4 root filesystem expanded successfully", "partition", rootPart, "output", strings.TrimSpace(string(rOut)))
		}
	}

	// Step 3: Write NoCloud seeds (meta-data, user-data, network-config)
	if err := WriteNoCloudSeeds(mountPoint, cfg, bootMAC); err != nil {
		return fmt.Errorf("failed injecting NoCloud seeds: %w", err)
	}

	// Step 7: Inject mdadm.conf if Software RAID is configured
	if cfg.Storage.RAIDLevel == domain.RAIDLevel1 || cfg.Storage.RAIDLevel == domain.RAIDLevel0 || cfg.Storage.RAIDLevel == domain.RAIDLevel10 || cfg.PartitioningPreset == domain.PartitioningRAID1 {
		if err := InjectMDADMConfig(ctx, mountPoint); err != nil {
			slog.WarnContext(ctx, "non-fatal warning injecting mdadm.conf into rootfs", "error", err)
		}
	}

	seedDir := filepath.Join(mountPoint, "var", "lib", "cloud", "seed", "nocloud")
	slog.InfoContext(ctx, "cloud-init nocloud seed injected successfully",
		"partition", rootPart,
		"seed_dir", seedDir,
		"boot_mac", bootMAC,
	)

	return nil
}

// WriteNoCloudSeeds generates and writes instance meta-data, user-data, and network-config into the target rootfs.
func WriteNoCloudSeeds(mountPoint string, cfg domain.DeploymentConfig, bootMAC string) error {
	seedDir := filepath.Join(mountPoint, "var", "lib", "cloud", "seed", "nocloud")
	if err := os.MkdirAll(seedDir, 0755); err != nil {
		return fmt.Errorf("failed creating seed directory %s: %w", seedDir, err)
	}

	hostname := fmt.Sprintf("node-%s", strings.ToLower(strings.ReplaceAll(bootMAC, ":", "")))
	metaDataContent := fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", cfg.NodeID, hostname)
	if err := os.WriteFile(filepath.Join(seedDir, "meta-data"), []byte(metaDataContent), 0644); err != nil {
		return fmt.Errorf("failed writing meta-data: %w", err)
	}

	userDataContent := generateUserData(cfg)
	if err := os.WriteFile(filepath.Join(seedDir, "user-data"), []byte(userDataContent), 0644); err != nil {
		return fmt.Errorf("failed writing user-data: %w", err)
	}

	networkConfigContent := generateNetworkConfig(cfg, bootMAC)
	if err := os.WriteFile(filepath.Join(seedDir, "network-config"), []byte(networkConfigContent), 0644); err != nil {
		return fmt.Errorf("failed writing network-config: %w", err)
	}

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
			hashedPass := crypto.HashSHA512Crypt(cfg.RootPassword, "")
			sb.WriteString("\nchpasswd:\n")
			sb.WriteString("  list: |\n")
			sb.WriteString(fmt.Sprintf("    root:%s\n", hashedPass))
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
		hashedPass := crypto.HashSHA512Crypt(cfg.RootPassword, "")
		sb.WriteString("\nchpasswd:\n")
		sb.WriteString("  list: |\n")
		sb.WriteString(fmt.Sprintf("    root:%s\n", hashedPass))
		sb.WriteString("  expire: false\n")
		sb.WriteString("ssh_pwauth: true\n")
	}

	sb.WriteString("\npackage_update: false\n")
	if cfg.Storage.RAIDLevel == domain.RAIDLevel1 || cfg.Storage.RAIDLevel == domain.RAIDLevel0 || cfg.Storage.RAIDLevel == domain.RAIDLevel10 || cfg.PartitioningPreset == domain.PartitioningRAID1 || cfg.PartitioningPreset == domain.PartitioningLVM || cfg.Storage.LayoutMode == domain.PartitioningLVM {
		sb.WriteString("packages:\n")
		sb.WriteString("  - mdadm\n")
		sb.WriteString("  - lvm2\n")
	}
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
	Name      string      `json:"name"`
	Path      string      `json:"path"`
	Size      json.Number `json:"size"`
	Type      string      `json:"type"`
	FSType    string      `json:"fstype"`
	Label     string      `json:"label"`
	PartLabel string      `json:"partlabel"`
	PartNum   json.Number `json:"partnum"`
	Children  []partInfo  `json:"children,omitempty"`
}

type partList struct {
	BlockDevices []partInfo `json:"blockdevices"`
}

func collectPartitions(devices []partInfo) []partInfo {
	var parts []partInfo
	for _, d := range devices {
		dev := d
		if dev.Path == "" {
			if strings.HasPrefix(dev.Name, "/") {
				dev.Path = dev.Name
			} else {
				dev.Path = "/dev/" + dev.Name
			}
		}
		if dev.Type == "part" {
			parts = append(parts, dev)
		}
		if len(dev.Children) > 0 {
			parts = append(parts, collectPartitions(dev.Children)...)
		}
	}
	return parts
}

func findRootPartition(ctx context.Context, targetDrivePath string) (string, error) {
	realDev, err := filepath.EvalSymlinks(targetDrivePath)
	if err != nil {
		realDev = targetDrivePath
	}

	cmd := exec.CommandContext(ctx, "lsblk", "-J", "-b", "-o", "NAME,PATH,SIZE,TYPE,FSTYPE,LABEL,PARTLABEL,PARTNUM", realDev)
	out, err := cmd.Output()
	if err != nil {
		return fallbackPartitionPath(realDev), nil
	}

	partPath, err := findRootPartitionFromJSON(out, realDev)
	if err != nil {
		return fallbackPartitionPath(realDev), nil
	}
	return partPath, nil
}

func findRootPartitionFromJSON(out []byte, realDev string) (string, error) {
	var data partList
	if err := json.Unmarshal(out, &data); err != nil {
		return "", err
	}

	parts := collectPartitions(data.BlockDevices)
	if len(parts) == 0 {
		return "", fmt.Errorf("no partitions detected")
	}

	// 1. Look for explicit root in PARTLABEL or LABEL (e.g. AlmaLinux PARTLABEL="root")
	for _, p := range parts {
		if strings.Contains(strings.ToLower(p.PartLabel), "root") || strings.Contains(strings.ToLower(p.Label), "root") {
			return p.Path, nil
		}
	}

	// 2. Look for largest xfs or ext4 root filesystem
	var candidate string
	var largestSize int64
	for _, p := range parts {
		if p.FSType == "xfs" || p.FSType == "ext4" {
			size, _ := p.Size.Int64()
			if size > largestSize {
				largestSize = size
				candidate = p.Path
			}
		}
	}
	if candidate != "" {
		return candidate, nil
	}

	// 3. Fallback to the largest partition overall
	for _, p := range parts {
		size, _ := p.Size.Int64()
		if size > largestSize {
			largestSize = size
			candidate = p.Path
		}
	}
	if candidate != "" {
		return candidate, nil
	}

	return fallbackPartitionPath(realDev), nil
}

func fallbackPartitionPath(realDev string) string {
	// Standard enterprise cloud images: partition 4 for AlmaLinux/RHEL (partition 1=biosboot, 2=ESP, 3=boot, 4=root)
	// Or partition 1 for Debian (partition 1=root, 15=ESP).
	part4 := resolvePartitionPath(realDev, 4)
	if _, err := os.Stat(part4); err == nil {
		return part4
	}
	part1 := resolvePartitionPath(realDev, 1)
	if _, err := os.Stat(part1); err == nil {
		return part1
	}
	return resolvePartitionPath(realDev, 4)
}

