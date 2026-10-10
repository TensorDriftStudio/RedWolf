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
	"gopkg.in/yaml.v3"
)

// InjectCloudInit writes NoCloud seed files directly to target rootfs by loop-mounting it in RAM.
func InjectCloudInit(ctx context.Context, targetDrivePath string, cfg domain.DeploymentConfig, bootMAC string) error {
	slog.InfoContext(ctx, "locating root partition for Cloud-Init NoCloud seed injection",
		"target_drive", targetDrivePath,
		"boot_mac", bootMAC,
		"os", cfg.OS,
	)

	// Detect root partition
	rootPart, err := findRootPartition(ctx, targetDrivePath, cfg.OS)
	if err != nil {
		return fmt.Errorf("failed detecting root partition on %s: %w", targetDrivePath, err)
	}

	mountPoint := "/mnt/redwolf-target"
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		return fmt.Errorf("failed creating mount point %s: %w", mountPoint, err)
	}

	// Ensure device nodes are settled before mounting
	_ = exec.CommandContext(ctx, "udevadm", "settle").Run()
	_ = exec.CommandContext(ctx, "mdev", "-s").Run()

	// Mount target root filesystem
	slog.InfoContext(ctx, "mounting root partition", "partition", rootPart, "mountpoint", mountPoint, "os", cfg.OS)
	if err := MountTargetFilesystem(ctx, rootPart, mountPoint, cfg.OS); err != nil {
		return fmt.Errorf("failed mounting partition %s to %s: %w", rootPart, mountPoint, err)
	}
	defer func() {
		umountCtx, umountCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer umountCancel()
		unmountAllUnder(umountCtx, mountPoint)
	}()

	// Mount separate /boot and ESP partitions if present (e.g. AlmaLinux / RHEL cloud images)
	bootMount := filepath.Join(mountPoint, "boot")
	_ = os.MkdirAll(bootMount, 0755)

	isDebianOrUbuntu := strings.Contains(strings.ToLower(string(cfg.OS)), "debian") || strings.Contains(strings.ToLower(string(cfg.OS)), "ubuntu")
	bootPartCandidate := resolvePartitionPath(targetDrivePath, 3)
	var bootMounted bool
	if !isDebianOrUbuntu && bootPartCandidate != rootPart {
		if _, err := os.Stat(bootPartCandidate); err == nil {
			if out, err := exec.CommandContext(ctx, "mount", bootPartCandidate, bootMount).CombinedOutput(); err == nil {
				bootMounted = true
				slog.InfoContext(ctx, "mounted separate boot partition", "partition", bootPartCandidate, "mountpoint", bootMount)
			} else {
				slog.DebugContext(ctx, "boot partition mount skipped", "output", string(out))
			}
		}
	}

	efiMount := filepath.Join(bootMount, "efi")
	_ = os.MkdirAll(efiMount, 0755)
	efiPartNum := detectEFIPartition(ctx, targetDrivePath, cfg.OS)
	efiPartCandidate := resolvePartitionPath(targetDrivePath, efiPartNum)
	var efiMounted bool
	if efiPartCandidate != rootPart && efiPartCandidate != bootPartCandidate {
		if _, err := os.Stat(efiPartCandidate); err == nil {
			if out, err := exec.CommandContext(ctx, "mount", "-t", "vfat", efiPartCandidate, efiMount).CombinedOutput(); err == nil {
				efiMounted = true
				slog.InfoContext(ctx, "mounted separate EFI system partition", "partition", efiPartCandidate, "mountpoint", efiMount)
			} else {
				slog.DebugContext(ctx, "efi partition mount skipped", "output", string(out))
			}
		}
	}

	// Online expand root filesystem to use the full partition capacity
	slog.InfoContext(ctx, "auto-expanding root filesystem to full partition capacity", "partition", rootPart, "mountpoint", mountPoint)
	xfsGrowCmd := exec.CommandContext(ctx, "xfs_growfs", mountPoint)
	if out, err := xfsGrowCmd.CombinedOutput(); err == nil {
		slog.InfoContext(ctx, "xfs root filesystem expanded successfully", "partition", rootPart, "output", strings.TrimSpace(string(out)))
	} else {
		resizeCmd := exec.CommandContext(ctx, "resize2fs", rootPart)
		if rOut, rErr := resizeCmd.CombinedOutput(); rErr == nil {
			slog.InfoContext(ctx, "ext4 root filesystem expanded successfully", "partition", rootPart, "output", strings.TrimSpace(string(rOut)))
		}
	}

	// Write NoCloud seeds (meta-data, user-data, network-config)
	if err := WriteNoCloudSeeds(ctx, mountPoint, cfg, bootMAC); err != nil {
		return fmt.Errorf("failed injecting NoCloud seeds: %w", err)
	}

	// Directly inject credentials into rootfs (/etc/shadow, /etc/ssh, /root/.ssh)
	if err := DirectInjectSecurityCredentials(ctx, mountPoint, cfg); err != nil {
		slog.WarnContext(ctx, "non-fatal warning directly injecting credentials into rootfs", "error", err)
	}

	// Inject mdadm.conf if Software RAID is configured
	if cfg.Storage.RAIDLevel == domain.RAIDLevel1 || cfg.Storage.RAIDLevel == domain.RAIDLevel0 || cfg.Storage.RAIDLevel == domain.RAIDLevel10 || cfg.PartitioningPreset == domain.PartitioningRAID1 {
		if err := InjectMDADMConfig(ctx, mountPoint); err != nil {
			slog.WarnContext(ctx, "non-fatal warning injecting mdadm.conf into rootfs", "error", err)
		}
	}

	// Install BIOS bootloader and generate universal GRUB config while target is mounted
	if err := InstallBIOSBootloader(ctx, []string{targetDrivePath}, bootMount, mountPoint, cfg.FirmwareMode); err != nil {
		slog.DebugContext(ctx, "warning during BIOS bootloader installation", "error", err)
	}

	layout := &StorageLayoutResult{
		TargetDrive:   targetDrivePath,
		RootPartition: rootPart,
	}
	if bootMounted {
		layout.BootPartition = bootPartCandidate
	}
	if efiMounted {
		layout.ESPPartition = efiPartCandidate
	}
	if err := GenerateUniversalGrubConfig(ctx, mountPoint, layout, cfg.OS); err != nil {
		slog.DebugContext(ctx, "non-fatal warning generating universal grub config", "error", err)
	}

	bootUUID := getPartitionUUID(ctx, layout.BootPartition)
	if bootUUID == "" {
		bootUUID = getPartitionUUID(ctx, layout.RootPartition)
	}
	_ = EnsureFallbackUEFILoader(ctx, mountPoint, bootUUID)

	seedDir := filepath.Join(mountPoint, "var", "lib", "cloud", "seed", "nocloud")
	slog.InfoContext(ctx, "cloud-init nocloud seed injected successfully",
		"partition", rootPart,
		"seed_dir", seedDir,
		"boot_mac", bootMAC,
	)

	return nil
}

// DirectInjectSecurityCredentials writes root credentials, authorized keys, and redwolf-release to rootfs.
func DirectInjectSecurityCredentials(ctx context.Context, mountPoint string, cfg domain.DeploymentConfig) error {
	var errs []string

	// Write /etc/redwolf-release
	_ = os.MkdirAll(filepath.Join(mountPoint, "etc"), 0755)
	releasePath := filepath.Join(mountPoint, "etc", "redwolf-release")
	releaseContent := fmt.Sprintf("RedWolf Bare-Metal Provisioning Engine\nNode ID: %s\nOperating System: %s\nProvisioned: %s\n",
		cfg.NodeID,
		cfg.OS,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err := os.WriteFile(releasePath, []byte(releaseContent), 0644); err != nil {
		errs = append(errs, fmt.Sprintf("failed writing /etc/redwolf-release: %v", err))
	} else {
		slog.InfoContext(ctx, "injected /etc/redwolf-release successfully into rootfs", "path", releasePath)
	}

	// Set root password hash in /etc/shadow
	if cfg.RootPassword != "" {
		shadowPath := filepath.Join(mountPoint, "etc", "shadow")
		hashedPass := crypto.HashSHA512Crypt(cfg.RootPassword, "")
		daysSinceEpoch := time.Now().Unix() / 86400

		if data, err := os.ReadFile(shadowPath); err == nil {
			lines := strings.Split(string(data), "\n")
			found := false
			for i, line := range lines {
				if strings.HasPrefix(line, "root:") {
					parts := strings.Split(line, ":")
					if len(parts) >= 2 {
						parts[1] = hashedPass
						if len(parts) > 2 && (parts[2] == "" || parts[2] == "0") {
							parts[2] = fmt.Sprintf("%d", daysSinceEpoch)
						}
						if len(parts) > 4 && parts[4] == "" {
							parts[4] = "99999"
						}
						lines[i] = strings.Join(parts, ":")
						found = true
						break
					}
				}
			}
			if !found {
				rootEntry := fmt.Sprintf("root:%s:%d:0:99999:7:::", hashedPass, daysSinceEpoch)
				lines = append(lines, rootEntry)
			}
			newContent := strings.Join(lines, "\n")
			if err := os.WriteFile(shadowPath, []byte(newContent), 0600); err != nil {
				errs = append(errs, fmt.Sprintf("failed writing /etc/shadow: %v", err))
			}
		} else {
			_ = os.MkdirAll(filepath.Join(mountPoint, "etc"), 0755)
			rootEntry := fmt.Sprintf("root:%s:%d:0:99999:7:::\n", hashedPass, daysSinceEpoch)
			if err := os.WriteFile(shadowPath, []byte(rootEntry), 0600); err != nil {
				errs = append(errs, fmt.Sprintf("failed creating /etc/shadow: %v", err))
			}
		}
	}

	// OpenSSH configuration for root login
	sshdDropinDir := filepath.Join(mountPoint, "etc", "ssh", "sshd_config.d")
	_ = os.MkdirAll(sshdDropinDir, 0755)
	dropinConf := "# RedWolf Provisioning Security Policy\nPermitRootLogin yes\nPasswordAuthentication yes\n"
	_ = os.WriteFile(filepath.Join(sshdDropinDir, "99-redwolf-root.conf"), []byte(dropinConf), 0644)

	sshdMainPath := filepath.Join(mountPoint, "etc", "ssh", "sshd_config")
	if data, err := os.ReadFile(sshdMainPath); err == nil {
		content := string(data)
		needsAppend := false
		if !strings.Contains(content, "sshd_config.d") {
			needsAppend = true
		}
		if needsAppend || !strings.Contains(content, "PermitRootLogin yes") {
			content += "\n# RedWolf Root Access\nPermitRootLogin yes\nPasswordAuthentication yes\n"
			_ = os.WriteFile(sshdMainPath, []byte(content), 0644)
		}
	}

	// SSH Public Keys injection
	if len(cfg.SSHKeys) > 0 {
		rootSSHDir := filepath.Join(mountPoint, "root", ".ssh")
		_ = os.MkdirAll(rootSSHDir, 0700)
		authKeysPath := filepath.Join(rootSSHDir, "authorized_keys")

		var validKeys []string
		for _, k := range cfg.SSHKeys {
			trimmed := strings.TrimSpace(k)
			if trimmed != "" {
				validKeys = append(validKeys, trimmed)
			}
		}
		if len(validKeys) > 0 {
			existing, _ := os.ReadFile(authKeysPath)
			existingContent := strings.TrimSpace(string(existing))
			var combined strings.Builder
			if existingContent != "" {
				combined.WriteString(existingContent)
				combined.WriteString("\n")
			}
			for _, k := range validKeys {
				if !strings.Contains(existingContent, k) {
					combined.WriteString(k)
					combined.WriteString("\n")
				}
			}
			_ = os.WriteFile(authKeysPath, []byte(combined.String()), 0600)
		}
	}

	// Trigger SELinux auto-relabel on first boot
	autorelabelPath := filepath.Join(mountPoint, ".autorelabel")
	_ = os.WriteFile(autorelabelPath, []byte(""), 0644)

	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// WriteNoCloudSeeds generates and writes instance meta-data, user-data, and network-config into the target rootfs.
func WriteNoCloudSeeds(ctx context.Context, mountPoint string, cfg domain.DeploymentConfig, bootMAC string) error {
	seedDirs := []string{
		filepath.Join(mountPoint, "var", "lib", "cloud", "seed", "nocloud"),
		filepath.Join(mountPoint, "var", "lib", "cloud", "seed", "nocloud-net"),
	}

	hostname := fmt.Sprintf("node-%s", strings.ToLower(strings.ReplaceAll(bootMAC, ":", "")))
	metaDataContent := fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", cfg.NodeID, hostname)
	userDataContent := generateUserData(cfg)
	networkConfigContent := generateNetworkConfig(cfg, bootMAC)

	for _, seedDir := range seedDirs {
		if err := os.MkdirAll(seedDir, 0755); err != nil {
			return fmt.Errorf("failed creating seed directory %s: %w", seedDir, err)
		}
		if err := os.WriteFile(filepath.Join(seedDir, "meta-data"), []byte(metaDataContent), 0644); err != nil {
			return fmt.Errorf("failed writing meta-data: %w", err)
		}
		if err := os.WriteFile(filepath.Join(seedDir, "user-data"), []byte(userDataContent), 0644); err != nil {
			return fmt.Errorf("failed writing user-data: %w", err)
		}
		if err := os.WriteFile(filepath.Join(seedDir, "network-config"), []byte(networkConfigContent), 0644); err != nil {
			return fmt.Errorf("failed writing network-config: %w", err)
		}
	}

	// Write Netplan config for Ubuntu and Debian distributions
	netplanDir := filepath.Join(mountPoint, "etc", "netplan")
	if _, err := os.Stat(netplanDir); err == nil || strings.Contains(strings.ToLower(string(cfg.OS)), "ubuntu") || strings.Contains(strings.ToLower(string(cfg.OS)), "debian") {
		_ = os.MkdirAll(netplanDir, 0755)
		_ = os.WriteFile(filepath.Join(netplanDir, "50-cloud-init.yaml"), []byte(networkConfigContent), 0600)
	}

	// Write NetworkManager keyfile fallback for AlmaLinux and RHEL distributions
	writeNetworkManagerFallback(mountPoint, cfg, bootMAC)

	// Force NoCloud datasource in cloud-init
	cloudCfgDir := filepath.Join(mountPoint, "etc", "cloud", "cloud.cfg.d")
	_ = os.MkdirAll(cloudCfgDir, 0755)
	dsConfig := "# RedWolf Provisioning Engine NoCloud Datasource Configuration\n" +
		"datasource_list: [ NoCloud, None ]\n" +
		"datasource:\n" +
		"  NoCloud:\n" +
		"    seed_dir: /var/lib/cloud/seed/nocloud\n" +
		"    fs_label: null\n"
	_ = os.WriteFile(filepath.Join(cloudCfgDir, "99-redwolf.cfg"), []byte(dsConfig), 0644)

	// Clean stale cloud-init artifacts from distro base image
	_ = os.Remove(filepath.Join(mountPoint, "etc", "cloud", "cloud-init.disabled"))
	_ = os.Remove(filepath.Join(mountPoint, "etc", "systemd", "system", "cloud-init.service"))
	_ = os.Remove(filepath.Join(mountPoint, "etc", "systemd", "system", "cloud-init-local.service"))
	_ = os.RemoveAll(filepath.Join(mountPoint, "var", "lib", "cloud", "instance"))
	_ = os.RemoveAll(filepath.Join(mountPoint, "var", "lib", "cloud", "instances"))
	_ = os.RemoveAll(filepath.Join(mountPoint, "var", "lib", "cloud", "data"))
	_ = os.RemoveAll(filepath.Join(mountPoint, "var", "lib", "cloud", "sem"))

	return nil
}

func writeNetworkManagerFallback(mountPoint string, cfg domain.DeploymentConfig, bootMAC string) {
	nmDir := filepath.Join(mountPoint, "etc", "NetworkManager", "system-connections")
	if _, err := os.Stat(nmDir); err != nil {
		if strings.Contains(strings.ToLower(string(cfg.OS)), "alma") || strings.Contains(strings.ToLower(string(cfg.OS)), "rhel") {
			_ = os.MkdirAll(nmDir, 0700)
		} else {
			return
		}
	}

	formattedMAC := strings.ToLower(strings.TrimSpace(bootMAC))
	var sb strings.Builder
	sb.WriteString("[connection]\nid=redwolf-boot\ntype=ethernet\nautoconnect=true\n\n")
	if formattedMAC != "" {
		sb.WriteString(fmt.Sprintf("[ethernet]\nmac-address=%s\n\n", formattedMAC))
	}
	sb.WriteString("[ipv4]\n")
	if cfg.NetworkMode == domain.NetworkModeStatic && cfg.StaticIP != "" {
		cidr := cfg.NetmaskCIDR
		if cidr <= 0 || cidr > 32 {
			cidr = 24
		}
		sb.WriteString("method=manual\n")
		if cfg.Gateway != "" {
			sb.WriteString(fmt.Sprintf("address1=%s/%d,%s\n", cfg.StaticIP, cidr, cfg.Gateway))
		} else {
			sb.WriteString(fmt.Sprintf("address1=%s/%d\n", cfg.StaticIP, cidr))
		}
		if len(cfg.DNSServers) > 0 {
			sb.WriteString(fmt.Sprintf("dns=%s;\n", strings.Join(cfg.DNSServers, ";")))
		}
	} else {
		sb.WriteString("method=auto\n")
	}
	sb.WriteString("\n[ipv6]\nmethod=ignore\n")

	filePath := filepath.Join(nmDir, "redwolf-boot.nmconnection")
	_ = os.WriteFile(filePath, []byte(sb.String()), 0600)
}

func hasTopLevelYAMLKey(content, key string) bool {
	var m map[string]any
	if err := yaml.Unmarshal([]byte(content), &m); err == nil && m != nil {
		_, ok := m[key]
		return ok
	}
	prefix := key + ":"
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimRight(line, " \t\r")
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func generateUserData(cfg domain.DeploymentConfig) string {
	if strings.TrimSpace(cfg.CustomUserData) != "" {
		base := strings.TrimSpace(cfg.CustomUserData)

		// Pass through shell scripts, boothooks, and MIME multipart payloads without modification
		if strings.HasPrefix(base, "#!") ||
			strings.HasPrefix(base, "Content-Type:") ||
			strings.HasPrefix(base, "#include") ||
			strings.HasPrefix(base, "#cloud-boothook") ||
			strings.HasPrefix(base, "#upstart-job") {
			return base + "\n"
		}

		if !strings.HasPrefix(base, "#cloud-config") {
			base = "#cloud-config\n" + base
		}

		hasUsers := hasTopLevelYAMLKey(base, "users")
		hasDisableRoot := hasTopLevelYAMLKey(base, "disable_root")
		hasSSHPwAuth := hasTopLevelYAMLKey(base, "ssh_pwauth")
		hasChpasswd := hasTopLevelYAMLKey(base, "chpasswd")
		hasPackages := hasTopLevelYAMLKey(base, "packages")

		var sb strings.Builder
		sb.WriteString(base)
		sb.WriteString("\n\n")

		if !hasDisableRoot {
			sb.WriteString("disable_root: false\n")
		}
		if !hasSSHPwAuth {
			sb.WriteString("ssh_pwauth: true\n")
		}

		// Inject root credentials only if user did not supply a custom users definition
		if !hasUsers {
			sb.WriteString("users:\n")
			sb.WriteString("  - name: root\n")
			sb.WriteString("    lock_passwd: false\n")
			if cfg.RootPassword != "" {
				hashedPass := crypto.HashSHA512Crypt(cfg.RootPassword, "")
				sb.WriteString(fmt.Sprintf("    passwd: \"%s\"\n", hashedPass))
			}
			if len(cfg.SSHKeys) > 0 {
				sb.WriteString("    ssh_authorized_keys:\n")
				for _, key := range cfg.SSHKeys {
					if trimmed := strings.TrimSpace(key); trimmed != "" {
						sb.WriteString(fmt.Sprintf("      - %s\n", trimmed))
					}
				}
			}
		}

		if !hasChpasswd && !hasUsers && cfg.RootPassword != "" {
			hashedPass := crypto.HashSHA512Crypt(cfg.RootPassword, "")
			sb.WriteString("\nchpasswd:\n")
			sb.WriteString("  list: |\n")
			sb.WriteString(fmt.Sprintf("    root:%s\n", hashedPass))
			sb.WriteString("  expire: false\n")
		}

		isRAIDOrLVM := cfg.Storage.RAIDLevel == domain.RAIDLevel1 ||
			cfg.Storage.RAIDLevel == domain.RAIDLevel0 ||
			cfg.Storage.RAIDLevel == domain.RAIDLevel10 ||
			cfg.PartitioningPreset == domain.PartitioningRAID1 ||
			cfg.PartitioningPreset == domain.PartitioningLVM ||
			cfg.Storage.LayoutMode == domain.PartitioningLVM

		if isRAIDOrLVM && !hasPackages {
			sb.WriteString("\npackages:\n")
			sb.WriteString("  - mdadm\n")
			sb.WriteString("  - lvm2\n")
		}

		return sb.String()
	}

	isRAIDOrLVM := cfg.Storage.RAIDLevel == domain.RAIDLevel1 ||
		cfg.Storage.RAIDLevel == domain.RAIDLevel0 ||
		cfg.Storage.RAIDLevel == domain.RAIDLevel10 ||
		cfg.PartitioningPreset == domain.PartitioningRAID1 ||
		cfg.PartitioningPreset == domain.PartitioningLVM ||
		cfg.Storage.LayoutMode == domain.PartitioningLVM

	var sb strings.Builder
	sb.WriteString("#cloud-config\n")
	sb.WriteString("disable_root: false\n")
	sb.WriteString("ssh_pwauth: true\n")
	if !isRAIDOrLVM {
		sb.WriteString("growpart:\n")
		sb.WriteString("  mode: auto\n")
		sb.WriteString("  devices: ['/']\n")
		sb.WriteString("  ignore_growpart_interface: false\n")
		sb.WriteString("resize_rootfs: true\n\n")
	} else {
		sb.WriteString("\n")
	}

	sb.WriteString("users:\n")
	sb.WriteString("  - name: root\n")
	sb.WriteString("    lock_passwd: false\n")
	if cfg.RootPassword != "" {
		hashedPass := crypto.HashSHA512Crypt(cfg.RootPassword, "")
		sb.WriteString(fmt.Sprintf("    passwd: \"%s\"\n", hashedPass))
	}
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
	}

	sb.WriteString("\npackage_update: false\n")
	if cfg.Storage.RAIDLevel == domain.RAIDLevel1 || cfg.Storage.RAIDLevel == domain.RAIDLevel0 || cfg.Storage.RAIDLevel == domain.RAIDLevel10 || cfg.PartitioningPreset == domain.PartitioningRAID1 || cfg.PartitioningPreset == domain.PartitioningLVM || cfg.Storage.LayoutMode == domain.PartitioningLVM {
		sb.WriteString("packages:\n")
		sb.WriteString("  - mdadm\n")
		sb.WriteString("  - lvm2\n")
	}
	sb.WriteString("\nwrite_files:\n")
	sb.WriteString("  - path: /etc/ssh/sshd_config.d/99-redwolf-root.conf\n")
	sb.WriteString("    permissions: '0644'\n")
	sb.WriteString("    content: |\n")
	sb.WriteString("      PermitRootLogin yes\n")
	sb.WriteString("      PasswordAuthentication yes\n")
	sb.WriteString("  - path: /etc/redwolf-release\n")
	sb.WriteString("    permissions: '0644'\n")
	sb.WriteString("    content: |\n")
	sb.WriteString("      RedWolf Bare-Metal Provisioning Engine\n")
	sb.WriteString("runcmd:\n")
	sb.WriteString("  - echo \"RedWolf bare-metal node initialized successfully\" > /etc/redwolf-release\n")

	return sb.String()
}

func generateNetworkConfig(cfg domain.DeploymentConfig, bootMAC string) string {
	if strings.TrimSpace(cfg.CustomNetworkConfig) != "" {
		return strings.TrimSpace(cfg.CustomNetworkConfig) + "\n"
	}

	var sb strings.Builder
	sb.WriteString("network:\n")
	sb.WriteString("  version: 2\n")

	formattedMAC := strings.ToLower(strings.TrimSpace(bootMAC))

	if cfg.EnableBonding {
		sb.WriteString("  ethernets:\n")
		sb.WriteString("    id0:\n")
		sb.WriteString("      match:\n")
		sb.WriteString(fmt.Sprintf("        macaddress: \"%s\"\n", formattedMAC))
		sb.WriteString("      set-name: eth0\n")
		sb.WriteString("  bonds:\n")
		sb.WriteString("    bond0:\n")
		sb.WriteString("      interfaces:\n")
		sb.WriteString("        - id0\n")
		sb.WriteString("      parameters:\n")
		sb.WriteString("        mode: 802.3ad\n")
		sb.WriteString("        lacp-rate: fast\n")
		sb.WriteString("        mii-monitor-interval: 100\n")

		targetIface := "bond0"
		if cfg.VLANTag > 0 {
			sb.WriteString("  vlans:\n")
			sb.WriteString(fmt.Sprintf("    vlan%d:\n", cfg.VLANTag))
			sb.WriteString(fmt.Sprintf("      id: %d\n", cfg.VLANTag))
			sb.WriteString("      link: bond0\n")
			targetIface = fmt.Sprintf("vlan%d", cfg.VLANTag)
		}
		appendInterfaceIPConfig(&sb, targetIface, cfg)
	} else if cfg.VLANTag > 0 {
		sb.WriteString("  ethernets:\n")
		sb.WriteString("    id0:\n")
		sb.WriteString("      match:\n")
		sb.WriteString(fmt.Sprintf("        macaddress: \"%s\"\n", formattedMAC))
		sb.WriteString("      set-name: eth0\n")
		sb.WriteString("  vlans:\n")
		sb.WriteString(fmt.Sprintf("    vlan%d:\n", cfg.VLANTag))
		sb.WriteString(fmt.Sprintf("      id: %d\n", cfg.VLANTag))
		sb.WriteString("      link: id0\n")
		appendInterfaceIPConfig(&sb, fmt.Sprintf("vlan%d", cfg.VLANTag), cfg)
	} else {
		sb.WriteString("  ethernets:\n")
		sb.WriteString("    id0:\n")
		sb.WriteString("      match:\n")
		sb.WriteString(fmt.Sprintf("        macaddress: \"%s\"\n", formattedMAC))
		sb.WriteString("      set-name: eth0\n")
		appendInterfaceIPConfig(&sb, "id0", cfg)
	}

	return sb.String()
}

func appendInterfaceIPConfig(sb *strings.Builder, iface string, cfg domain.DeploymentConfig) {
	indent := "      "
	if iface == "id0" {
		indent = "      "
	}

	if cfg.NetworkMode == domain.NetworkModeStatic {
		sb.WriteString(fmt.Sprintf("%sdhcp4: false\n", indent))
		sb.WriteString(fmt.Sprintf("%sdhcp6: false\n", indent))
		cidr := cfg.NetmaskCIDR
		if cidr <= 0 || cidr > 32 {
			cidr = 24
		}
		sb.WriteString(fmt.Sprintf("%saddresses:\n", indent))
		sb.WriteString(fmt.Sprintf("%s  - %s/%d\n", indent, cfg.StaticIP, cidr))

		if cfg.Gateway != "" {
			sb.WriteString(fmt.Sprintf("%sroutes:\n", indent))
			sb.WriteString(fmt.Sprintf("%s  - to: default\n", indent))
			sb.WriteString(fmt.Sprintf("%s    via: %s\n", indent, cfg.Gateway))
		}

		if len(cfg.DNSServers) > 0 {
			sb.WriteString(fmt.Sprintf("%snameservers:\n", indent))
			sb.WriteString(fmt.Sprintf("%s  addresses:\n", indent))
			for _, dns := range cfg.DNSServers {
				sb.WriteString(fmt.Sprintf("%s    - %s\n", indent, dns))
			}
		} else {
			sb.WriteString(fmt.Sprintf("%snameservers:\n", indent))
			sb.WriteString(fmt.Sprintf("%s  addresses:\n", indent))
			sb.WriteString(fmt.Sprintf("%s    - 1.1.1.1\n", indent))
			sb.WriteString(fmt.Sprintf("%s    - 8.8.8.8\n", indent))
		}
	} else {
		sb.WriteString(fmt.Sprintf("%sdhcp4: true\n", indent))
		sb.WriteString(fmt.Sprintf("%sdhcp6: false\n", indent))
	}
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

func findRootPartition(ctx context.Context, targetDrivePath string, osType ...domain.OperatingSystem) (string, error) {
	realDev, err := filepath.EvalSymlinks(targetDrivePath)
	if err != nil {
		realDev = targetDrivePath
	}

	var targetOS domain.OperatingSystem
	if len(osType) > 0 {
		targetOS = osType[0]
	}

	cmd := exec.CommandContext(ctx, "lsblk", "-J", "-b", "-o", "NAME,PATH,SIZE,TYPE,FSTYPE,LABEL,PARTLABEL,PARTNUM", realDev)
	out, err := cmd.Output()
	if err != nil {
		return fallbackPartitionPath(realDev, targetOS), nil
	}

	partPath, err := findRootPartitionFromJSON(out, realDev, targetOS)
	if err != nil {
		return fallbackPartitionPath(realDev, targetOS), nil
	}
	return partPath, nil
}

func findRootPartitionFromJSON(out []byte, realDev string, osType ...domain.OperatingSystem) (string, error) {
	var targetOS domain.OperatingSystem
	if len(osType) > 0 {
		targetOS = osType[0]
	}
	isDebian := strings.Contains(strings.ToLower(string(targetOS)), "debian") || strings.Contains(strings.ToLower(string(targetOS)), "ubuntu")
	isRHEL := strings.Contains(strings.ToLower(string(targetOS)), "alma") || strings.Contains(strings.ToLower(string(targetOS)), "rhel") || strings.Contains(strings.ToLower(string(targetOS)), "centos") || strings.Contains(strings.ToLower(string(targetOS)), "rocky")

	var data partList
	if err := json.Unmarshal(out, &data); err != nil {
		return "", err
	}

	parts := collectPartitions(data.BlockDevices)
	if len(parts) == 0 {
		return "", fmt.Errorf("no partitions detected")
	}

	// 1. If Debian, partition 1 is unequivocally the root partition in GenericCloud images
	if isDebian {
		for _, p := range parts {
			num := extractTrailingDigits(p.Name)
			sz, _ := p.Size.Int64()
			if (num == 1 || strings.HasSuffix(p.Path, "p1") || strings.HasSuffix(p.Path, "1")) && (sz == 0 || sz > 100*1024*1024) {
				return p.Path, nil
			}
		}
		for _, p := range parts {
			if p.FSType == "ext4" {
				return p.Path, nil
			}
		}
		return fallbackPartitionPath(realDev, targetOS), nil
	}

	// 2. Look for explicit root in PARTLABEL or LABEL with a verified filesystem
	for _, p := range parts {
		if strings.Contains(strings.ToLower(p.PartLabel), "root") || strings.Contains(strings.ToLower(p.Label), "root") {
			if p.FSType == "xfs" || p.FSType == "ext4" || p.FSType == "btrfs" {
				return p.Path, nil
			}
		}
	}

	// 3. For RHEL/AlmaLinux, look for non-boot XFS root filesystem first
	if isRHEL {
		var xfsCandidate string
		var largestXFS int64
		for _, p := range parts {
			if p.FSType == "xfs" {
				if strings.EqualFold(p.Label, "boot") || strings.EqualFold(p.PartLabel, "boot") ||
					strings.Contains(strings.ToLower(p.Label), "efi") || strings.Contains(strings.ToLower(p.PartLabel), "efi") {
					continue
				}
				sz, _ := p.Size.Int64()
				if sz > largestXFS {
					largestXFS = sz
					xfsCandidate = p.Path
				}
			}
		}
		if xfsCandidate != "" {
			return xfsCandidate, nil
		}

		// Check explicit root label
		for _, p := range parts {
			if strings.Contains(strings.ToLower(p.PartLabel), "root") || strings.Contains(strings.ToLower(p.Label), "root") {
				return p.Path, nil
			}
		}

		// Partition 4 standard in AlmaLinux 8/9 cloud images
		for _, p := range parts {
			num := extractTrailingDigits(p.Name)
			if num == 4 {
				return p.Path, nil
			}
		}
	}

	// 4. Look for largest xfs or ext4 root filesystem (excluding /boot or ESP partitions)
	var candidate string
	var largestSize int64
	for _, p := range parts {
		if p.FSType == "xfs" || p.FSType == "ext4" || p.FSType == "btrfs" {
			if strings.EqualFold(p.Label, "boot") || strings.EqualFold(p.PartLabel, "boot") ||
				strings.Contains(strings.ToLower(p.Label), "efi") || strings.Contains(strings.ToLower(p.PartLabel), "efi") {
				continue
			}
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

	// 5. Look for explicit root in PARTLABEL or LABEL (even without recognized fstype in synthetic test mocks)
	for _, p := range parts {
		if strings.Contains(strings.ToLower(p.PartLabel), "root") || strings.Contains(strings.ToLower(p.Label), "root") {
			return p.Path, nil
		}
	}

	// 6. For AlmaLinux/RHEL standard cloud images, look for partition 4
	for _, p := range parts {
		num := extractTrailingDigits(p.Name)
		if num == 4 {
			return p.Path, nil
		}
	}

	// 7. Look for any largest xfs/ext4 partition overall
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

	// 8. Fallback to the largest partition overall
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

	return fallbackPartitionPath(realDev, targetOS), nil
}

func fallbackPartitionPath(realDev string, osType ...domain.OperatingSystem) string {
	var targetOS domain.OperatingSystem
	if len(osType) > 0 {
		targetOS = osType[0]
	}
	isDebianLike := strings.Contains(strings.ToLower(string(targetOS)), "debian") || strings.Contains(strings.ToLower(string(targetOS)), "ubuntu")

	if isDebianLike {
		part1 := resolvePartitionPath(realDev, 1)
		if _, err := os.Stat(part1); err == nil {
			return part1
		}
		return resolvePartitionPath(realDev, 1)
	}

	// Standard enterprise cloud images: partition 4 for AlmaLinux/RHEL (partition 1=biosboot, 2=ESP, 3=boot, 4=root)
	part4 := resolvePartitionPath(realDev, 4)
	if _, err := os.Stat(part4); err == nil {
		return part4
	}
	part3 := resolvePartitionPath(realDev, 3)
	if _, err := os.Stat(part3); err == nil {
		return part3
	}
	part1 := resolvePartitionPath(realDev, 1)
	if _, err := os.Stat(part1); err == nil {
		return part1
	}
	return resolvePartitionPath(realDev, 4)
}

// MountTargetFilesystem mounts a target root/boot filesystem using robust filesystem detection,
// explicit filesystem type flags (-t xfs, -t ext4, -t btrfs), and dynamic kernel module loading.
// This prevents "no valid filesystem type specified" errors on minimal BusyBox initramfs environments.
func MountTargetFilesystem(ctx context.Context, partPath string, mountPoint string, targetOS domain.OperatingSystem, extraArgs ...string) error {
	// 1. Proactively ensure filesystem kernel modules are loaded
	_ = exec.CommandContext(ctx, "modprobe", "xfs").Run()
	_ = exec.CommandContext(ctx, "modprobe", "ext4").Run()
	_ = exec.CommandContext(ctx, "modprobe", "btrfs").Run()

	// 2. Ensure /etc/filesystems exists with enterprise filesystem search order for BusyBox mount
	_ = os.WriteFile("/etc/filesystems", []byte("ext4\nxfs\nbtrfs\nvfat\n*\n"), 0644)

	// Settle devices before probing and mounting
	settlePartitions(ctx, partPath)

	// 3. Detect filesystem type via blkid / lsblk
	detectedFSType := probeDeviceFSType(ctx, partPath)

	// 4. Assemble candidate filesystem types in order of likelihood
	var candidates []string
	if detectedFSType != "" {
		candidates = append(candidates, detectedFSType)
	}

	isDebian := strings.Contains(strings.ToLower(string(targetOS)), "debian") || strings.Contains(strings.ToLower(string(targetOS)), "ubuntu")
	isRHEL := strings.Contains(strings.ToLower(string(targetOS)), "alma") || strings.Contains(strings.ToLower(string(targetOS)), "rhel") || strings.Contains(strings.ToLower(string(targetOS)), "centos") || strings.Contains(strings.ToLower(string(targetOS)), "rocky")

	if isRHEL {
		candidates = append(candidates, "xfs", "ext4", "btrfs")
	} else if isDebian {
		candidates = append(candidates, "ext4", "xfs", "btrfs")
	} else {
		candidates = append(candidates, "xfs", "ext4", "btrfs")
	}
	candidates = append(candidates, "") // Fallback to auto-mount without -t

	// Deduplicate candidates preserving priority order
	seen := make(map[string]bool)
	var uniqueCandidates []string
	for _, c := range candidates {
		if !seen[c] {
			seen[c] = true
			uniqueCandidates = append(uniqueCandidates, c)
		}
	}

	var lastErr error
	var lastOutput string
	for _, fs := range uniqueCandidates {
		var args []string
		args = append(args, extraArgs...)
		if fs != "" {
			args = append(args, "-t", fs)
		}
		args = append(args, partPath, mountPoint)

		mountCmd := exec.CommandContext(ctx, "mount", args...)
		out, err := mountCmd.CombinedOutput()
		if err == nil {
			slog.InfoContext(ctx, "target filesystem mounted successfully",
				"partition", partPath,
				"mountpoint", mountPoint,
				"fstype", fs,
				"detected_type", detectedFSType,
			)
			return nil
		}
		lastErr = err
		lastOutput = strings.TrimSpace(string(out))
		slog.DebugContext(ctx, "candidate mount attempt failed",
			"partition", partPath,
			"fstype", fs,
			"error", err,
			"output", lastOutput,
		)
	}

	return fmt.Errorf("failed mounting partition %s to %s with attempted filesystems %v: %w (output: %s)",
		partPath, mountPoint, uniqueCandidates, lastErr, lastOutput)
}

func probeDeviceFSType(ctx context.Context, devicePath string) string {
	// First try blkid for direct superblock inspection
	cmd := exec.CommandContext(ctx, "blkid", "-s", "TYPE", "-o", "value", devicePath)
	if out, err := cmd.Output(); err == nil {
		fs := strings.TrimSpace(string(out))
		if fs != "" {
			return fs
		}
	}

	// Fallback to lsblk
	lsCmd := exec.CommandContext(ctx, "lsblk", "-n", "-o", "FSTYPE", devicePath)
	if out, err := lsCmd.Output(); err == nil {
		fs := strings.TrimSpace(string(out))
		if fs != "" {
			return fs
		}
	}

	return ""
}


