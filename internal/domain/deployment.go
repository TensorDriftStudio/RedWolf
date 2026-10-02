package domain

import (
	"fmt"
	"net"
	"strings"
)

// OperatingSystem identifies supported target distributions.
type OperatingSystem string

const (
	OSAlmaLinux9 OperatingSystem = "AlmaLinux 9"
	OSAlmaLinux8 OperatingSystem = "AlmaLinux 8"
	OSDebian12   OperatingSystem = "Debian 12"
	OSAlmaLinux10 OperatingSystem = "AlmaLinux 10"
	OSDebian13   OperatingSystem = "Debian 13"
)

// PartitioningPreset defines the storage layout pattern.
type PartitioningPreset string

const (
	PartitioningStandard PartitioningPreset = "standard"
	PartitioningLVM      PartitioningPreset = "lvm"
	PartitioningRAID1    PartitioningPreset = "raid1"
)

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
	TemplateID         string             `json:"templateId,omitempty"`
	CustomUserData     string             `json:"customUserData,omitempty"`
}

// Validate verifies deployment parameters against enterprise invariants.
func (cfg *DeploymentConfig) Validate() error {
	if cfg.NodeID == "" {
		return fmt.Errorf("nodeId is required")
	}

	switch cfg.OS {
	case OSAlmaLinux9, OSAlmaLinux8, OSDebian12, OSAlmaLinux10, OSDebian13:
		// Valid distribution
	default:
		return fmt.Errorf("unsupported operating system: %s", cfg.OS)
	}

	if strings.TrimSpace(cfg.TargetDrivePath) == "" {
		return ErrInvalidStorageDevice
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
