package domain

import (
	"fmt"
	"net"
	"strings"

	"gopkg.in/yaml.v3"
)

// OperatingSystem identifies supported target distributions.
type OperatingSystem string

const (
	OSAlmaLinux9  OperatingSystem = "AlmaLinux 9"
	OSAlmaLinux8  OperatingSystem = "AlmaLinux 8"
	OSDebian12    OperatingSystem = "Debian 12"
	OSAlmaLinux10 OperatingSystem = "AlmaLinux 10"
	OSDebian13    OperatingSystem = "Debian 13"
	OSUbuntu2404  OperatingSystem = "Ubuntu 24.04 LTS"
	OSUbuntu2204  OperatingSystem = "Ubuntu 22.04 LTS"
)

// PartitioningPreset defines the storage layout pattern.
type PartitioningPreset string

const (
	PartitioningStandard PartitioningPreset = "standard"
	PartitioningLVM      PartitioningPreset = "lvm"
	PartitioningRAID1    PartitioningPreset = "raid1"
)

// RAIDLevel specifies Software RAID topology.
type RAIDLevel string

const (
	RAIDLevelNone RAIDLevel = "none"
	RAIDLevel0    RAIDLevel = "raid0"
	RAIDLevel1    RAIDLevel = "raid1"
	RAIDLevel10   RAIDLevel = "raid10"
)

// LVMVolumeConfig specifies a logical volume allocation.
type LVMVolumeConfig struct {
	Name       string `json:"name"`
	MountPoint string `json:"mountPoint"`
	SizeGB     int    `json:"sizeGb"`
	FSType     string `json:"fsType"` // "xfs", "ext4"
}

// StorageConfig encapsulates advanced multi-disk, RAID, and LVM configuration.
type StorageConfig struct {
	LayoutMode   PartitioningPreset `json:"layoutMode"`             // "standard", "lvm", "raid1"
	RAIDLevel    RAIDLevel          `json:"raidLevel,omitempty"`    // "none", "raid0", "raid1", "raid10"
	TargetDrives []string           `json:"targetDrives,omitempty"` // Multi-disk targets
	LVMVolumes   []LVMVolumeConfig  `json:"lvmVolumes,omitempty"`
	SwapSizeGB   int                `json:"swapSizeGb,omitempty"`
}

// NetworkMode specifies IP allocation strategy.
type NetworkMode string

const (
	NetworkModeStatic NetworkMode = "static"
	NetworkModeDHCP   NetworkMode = "dhcp"
)

// DeploymentConfig defines the parameters for bare-metal OS provisioning.
type DeploymentConfig struct {
	NodeID             string             `json:"nodeId"`
	OS                 OperatingSystem    `json:"os"`
	TargetDrivePath    string             `json:"targetDrivePath"`
	PartitioningPreset PartitioningPreset `json:"partitioningPreset"`
	Storage            StorageConfig      `json:"storage,omitempty"`
	RootPassword       string             `json:"rootPassword"`
	SSHKeys            []string           `json:"sshKeys"`
	NetworkMode        NetworkMode        `json:"networkMode"`
	StaticIP           string             `json:"staticIp,omitempty"`
	NetmaskCIDR        int                `json:"netmaskCidr,omitempty"`
	Gateway            string             `json:"gateway,omitempty"`
	DNSServers         []string           `json:"dnsServers,omitempty"`
	VLANTag            int                `json:"vlanTag,omitempty"`
	EnableBonding      bool               `json:"enableBonding"`
	BondInterfaces     []string           `json:"bondInterfaces,omitempty"`
	TemplateID          string             `json:"templateId,omitempty"`
	CustomUserData      string             `json:"customUserData,omitempty"`
	CustomNetworkConfig string             `json:"customNetworkConfig,omitempty"`
	FirmwareMode        FirmwareMode       `json:"firmwareMode,omitempty"`
}

// Validate verifies deployment parameters against enterprise invariants.
func (cfg *DeploymentConfig) Validate() error {
	if cfg.NodeID == "" {
		return fmt.Errorf("nodeId is required")
	}

	if cfg.FirmwareMode == "" {
		cfg.FirmwareMode = FirmwareAuto
	}

	normalizedOS := strings.ToLower(string(cfg.OS))
	normalizedOS = strings.ReplaceAll(normalizedOS, " ", "")
	normalizedOS = strings.ReplaceAll(normalizedOS, "-", "")
	normalizedOS = strings.ReplaceAll(normalizedOS, ".", "")

	switch normalizedOS {
	case "almalinux9":
		cfg.OS = OSAlmaLinux9
	case "almalinux8":
		cfg.OS = OSAlmaLinux8
	case "almalinux10":
		cfg.OS = OSAlmaLinux10
	case "debian12":
		cfg.OS = OSDebian12
	case "debian13":
		cfg.OS = OSDebian13
	case "ubuntu2404", "ubuntu2404lts", "ubuntu24", "ubuntunoble", "noble":
		cfg.OS = OSUbuntu2404
	case "ubuntu2204", "ubuntu2204lts", "ubuntu22", "ubuntujammy", "jammy":
		cfg.OS = OSUbuntu2204
	case "ubuntu", "ubuntults":
		cfg.OS = OSUbuntu2404 // Default Ubuntu to current LTS
	default:
		return fmt.Errorf("unsupported operating system: %s", cfg.OS)
	}

	// Synchronize target drives
	if len(cfg.Storage.TargetDrives) == 0 && strings.TrimSpace(cfg.TargetDrivePath) != "" {
		cfg.Storage.TargetDrives = []string{strings.TrimSpace(cfg.TargetDrivePath)}
	} else if len(cfg.Storage.TargetDrives) > 0 && strings.TrimSpace(cfg.TargetDrivePath) == "" {
		cfg.TargetDrivePath = cfg.Storage.TargetDrives[0]
	}

	if len(cfg.Storage.TargetDrives) == 0 {
		return ErrInvalidStorageDevice
	}

	// Validate Software RAID requirements
	if cfg.PartitioningPreset == PartitioningRAID1 && (cfg.Storage.RAIDLevel == "" || cfg.Storage.RAIDLevel == RAIDLevelNone) {
		cfg.Storage.RAIDLevel = RAIDLevel1
	}

	switch cfg.Storage.RAIDLevel {
	case RAIDLevel1:
		if len(cfg.Storage.TargetDrives) < 2 {
			return fmt.Errorf("software RAID 1 requires at least 2 target storage drives (selected %d)", len(cfg.Storage.TargetDrives))
		}
	case RAIDLevel0:
		if len(cfg.Storage.TargetDrives) < 2 {
			return fmt.Errorf("software RAID 0 requires at least 2 target storage drives (selected %d)", len(cfg.Storage.TargetDrives))
		}
	case RAIDLevel10:
		if len(cfg.Storage.TargetDrives) < 4 {
			return fmt.Errorf("software RAID 10 requires at least 4 target storage drives (selected %d)", len(cfg.Storage.TargetDrives))
		}
	}

	// Validate LVM Volume requirements
	if cfg.Storage.LayoutMode == PartitioningLVM || cfg.PartitioningPreset == PartitioningLVM {
		fillCount := 0
		for _, vol := range cfg.Storage.LVMVolumes {
			if strings.TrimSpace(vol.Name) == "" {
				return fmt.Errorf("lvm volume name cannot be empty")
			}
			if vol.SizeGB < 0 {
				return fmt.Errorf("lvm volume %s size cannot be negative", vol.Name)
			}
			if vol.SizeGB == 0 {
				fillCount++
			}
		}
		if fillCount > 1 {
			return fmt.Errorf("at most one LVM volume may specify size 0 (fill remaining space)")
		}
	}

	// Validate Custom Cloud-Init YAML formatting
	if strings.TrimSpace(cfg.CustomUserData) != "" {
		trimmed := strings.TrimSpace(cfg.CustomUserData)
		// Scripts and multipart directives are exempt from pure YAML parsing
		if !strings.HasPrefix(trimmed, "#!") &&
			!strings.HasPrefix(trimmed, "Content-Type:") &&
			!strings.HasPrefix(trimmed, "#include") &&
			!strings.HasPrefix(trimmed, "#cloud-boothook") &&
			!strings.HasPrefix(trimmed, "#upstart-job") {
			var node yaml.Node
			if err := yaml.Unmarshal([]byte(trimmed), &node); err != nil {
				return fmt.Errorf("%w: user-data parsing failed: %v", ErrInvalidYAMLConfig, err)
			}
		}
	}

	if strings.TrimSpace(cfg.CustomNetworkConfig) != "" {
		var node yaml.Node
		if err := yaml.Unmarshal([]byte(cfg.CustomNetworkConfig), &node); err != nil {
			return fmt.Errorf("%w: network-config parsing failed: %v", ErrInvalidYAMLConfig, err)
		}
	}

	if len(cfg.RootPassword) < 8 && len(cfg.SSHKeys) == 0 {
		return ErrMissingCredentials
	}

	if cfg.NetworkMode == NetworkModeStatic {
		if net.ParseIP(cfg.StaticIP) == nil {
			return fmt.Errorf("%w: invalid static IP: %s", ErrInvalidIPAddress, cfg.StaticIP)
		}
		if cfg.Gateway != "" && net.ParseIP(cfg.Gateway) == nil {
			return fmt.Errorf("%w: invalid gateway: %s", ErrInvalidIPAddress, cfg.Gateway)
		}
	}

	return nil
}

// DeploymentTask encapsulates the complete execution payload dispatched to the discovery agent.
type DeploymentTask struct {
	TaskID          string           `json:"taskId"`
	NodeID          string           `json:"nodeId"`
	OS              OperatingSystem  `json:"os"`
	ImageURL        string           `json:"imageUrl"`
	TargetDrivePath string           `json:"targetDrivePath"`
	Config          DeploymentConfig `json:"config"`
}
