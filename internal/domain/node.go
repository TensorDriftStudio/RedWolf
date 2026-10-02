package domain

import (
	"fmt"
	"time"
)

// NodeStatus defines the lifecycle finite state machine of a server node.
type NodeStatus string

const (
	NodeStatusDiscovering  NodeStatus = "DISCOVERING"
	NodeStatusReady        NodeStatus = "READY_FOR_PROVISIONING"
	NodeStatusProvisioning NodeStatus = "PROVISIONING"
	NodeStatusActive       NodeStatus = "ACTIVE"
	NodeStatusError        NodeStatus = "ERROR"
)

// ProvisioningState captures live progress of active deployment operations.
type ProvisioningState struct {
	Progress    int      `json:"progress"`
	Stage       string   `json:"stage"`
	OS          string   `json:"os"`
	TargetDrive string   `json:"targetDrive"`
	TargetIP    string   `json:"targetIp"`
	Logs        []string `json:"logs"`
}

// ServerNode represents a managed physical bare-metal or virtual server.
type ServerNode struct {
	ID                string             `json:"id"`
	Vendor            Vendor             `json:"vendor"`
	Model             string             `json:"model"`
	SerialNumber      string             `json:"serialNumber"`
	FirmwareMode      FirmwareMode       `json:"firmwareMode"`
	BIOSVersion       string             `json:"biosVersion"`
	Status            NodeStatus         `json:"status"`
	CPU               CPUInfo            `json:"cpu"`
	Memory            MemoryInfo         `json:"memory"`
	Storage           []StorageDevice    `json:"storage"`
	NICs              []NetworkInterface `json:"nics"`
	BMC               BMCInfo            `json:"bmc"`
	ProvisioningState *ProvisioningState `json:"provisioningState,omitempty"`
	DiscoveredAt      time.Time          `json:"discoveredAt"`
	UpdatedAt         time.Time          `json:"updatedAt"`
}

// BootMAC returns the MAC address of the primary boot network interface.
func (n *ServerNode) BootMAC() string {
	for _, nic := range n.NICs {
		if nic.IsBoot {
			return nic.MAC
		}
	}
	if len(n.NICs) > 0 {
		return n.NICs[0].MAC
	}
	return ""
}

// CanTransitionTo enforces valid FSM state machine progression.
func (n *ServerNode) CanTransitionTo(next NodeStatus) bool {
	switch n.Status {
	case NodeStatusDiscovering:
		return next == NodeStatusReady || next == NodeStatusError
	case NodeStatusReady:
		return next == NodeStatusProvisioning || next == NodeStatusError
	case NodeStatusProvisioning:
		return next == NodeStatusActive || next == NodeStatusError || next == NodeStatusReady
	case NodeStatusActive:
		return next == NodeStatusProvisioning || next == NodeStatusReady || next == NodeStatusError
	case NodeStatusError:
		return next == NodeStatusReady || next == NodeStatusDiscovering || next == NodeStatusProvisioning
	default:
		return false
	}
}

// TransitionTo validates and applies a new state to the node.
func (n *ServerNode) TransitionTo(next NodeStatus) error {
	if !n.CanTransitionTo(next) {
		return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidNodeStatus, n.Status, next)
	}
	n.Status = next
	n.UpdatedAt = time.Now().UTC()
	return nil
}
