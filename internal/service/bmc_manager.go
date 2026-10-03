package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/tensordriftstudio/redwolf/internal/adapter/ipmi"
	"github.com/tensordriftstudio/redwolf/internal/adapter/redfish"
	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/port"
)

// BMCManager coordinates out-of-band server control using Redfish with IPMI RMCP+ fallback.
type BMCManager struct {
	repo    port.NodeRepository
	escrow  *BMCEscrowService
	redfish port.BMCController
	ipmi    port.BMCController
}

// NewBMCManager initializes the BMC management coordinator.
func NewBMCManager(repo port.NodeRepository, escrow *BMCEscrowService) *BMCManager {
	return &BMCManager{
		repo:    repo,
		escrow:  escrow,
		redfish: redfish.NewClient(),
		ipmi:    ipmi.NewRemoteController(),
	}
}

// GetNodePowerState queries the physical power state of a node's chassis.
func (m *BMCManager) GetNodePowerState(ctx context.Context, nodeID string) (*domain.PowerStatusResponse, error) {
	node, err := m.repo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("node %s not found: %w", nodeID, err)
	}

	if node.BMC.IP == "" {
		return &domain.PowerStatusResponse{
			NodeID:     nodeID,
			PowerState: domain.PowerStateUnknown,
			BMCIP:      "",
		}, nil
	}

	creds, _ := m.escrow.GetCredentials(ctx, nodeID)

	// Try Redfish first
	state, err := m.redfish.GetPowerState(ctx, node.BMC, creds)
	if err == nil && state != domain.PowerStateUnknown {
		return &domain.PowerStatusResponse{
			NodeID:     nodeID,
			PowerState: state,
			BMCIP:      node.BMC.IP,
		}, nil
	}

	// Fallback to IPMI 2.0 RMCP+
	ipmiState, ipmiErr := m.ipmi.GetPowerState(ctx, node.BMC, creds)
	if ipmiErr == nil {
		return &domain.PowerStatusResponse{
			NodeID:     nodeID,
			PowerState: ipmiState,
			BMCIP:      node.BMC.IP,
		}, nil
	}

	slog.WarnContext(ctx, "could not query chassis power state via redfish or ipmi",
		"node_id", nodeID,
		"bmc_ip", node.BMC.IP,
		"redfish_err", err,
		"ipmi_err", ipmiErr,
	)

	return &domain.PowerStatusResponse{
		NodeID:     nodeID,
		PowerState: domain.PowerStateUnknown,
		BMCIP:      node.BMC.IP,
	}, nil
}

// ExecuteNodePowerAction sends a verified power control command to a node's BMC.
func (m *BMCManager) ExecuteNodePowerAction(ctx context.Context, nodeID string, action domain.PowerAction) error {
	if err := action.Validate(); err != nil {
		return err
	}

	node, err := m.repo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("node %s not found: %w", nodeID, err)
	}

	if node.BMC.IP == "" {
		return fmt.Errorf("node %s has no registered BMC IP address", nodeID)
	}

	creds, _ := m.escrow.GetCredentials(ctx, nodeID)

	slog.InfoContext(ctx, "executing out-of-band power action",
		"node_id", nodeID,
		"action", action,
		"bmc_ip", node.BMC.IP,
	)

	// Try Redfish first
	if err := m.redfish.ExecutePowerAction(ctx, node.BMC, creds, action); err == nil {
		slog.InfoContext(ctx, "power action executed via redfish successfully", "node_id", nodeID, "action", action)
		return nil
	} else {
		slog.WarnContext(ctx, "redfish power action failed; attempting ipmi fallback", "node_id", nodeID, "error", err)
	}

	// Fallback to IPMI 2.0 RMCP+
	if err := m.ipmi.ExecutePowerAction(ctx, node.BMC, creds, action); err != nil {
		return fmt.Errorf("both redfish and ipmi power actions failed: %w", err)
	}

	slog.InfoContext(ctx, "power action executed via ipmi rmcp+ successfully", "node_id", nodeID, "action", action)
	return nil
}

// SynchronizeCredentials applies updated credentials to the physical BMC hardware via Redfish or IPMI RMCP+.
func (m *BMCManager) SynchronizeCredentials(ctx context.Context, nodeID string, newUsername, newPassword string, slot int) error {
	node, err := m.repo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("node %s not found: %w", nodeID, err)
	}

	if node.BMC.IP == "" {
		return fmt.Errorf("node %s has no registered BMC IP address", nodeID)
	}

	currentCreds, _ := m.escrow.GetCredentials(ctx, nodeID)

	slog.InfoContext(ctx, "synchronizing credentials to physical bmc hardware",
		"node_id", nodeID,
		"bmc_ip", node.BMC.IP,
		"slot", slot,
		"username", newUsername,
	)

	// Try Redfish first
	if err := m.redfish.UpdateCredentials(ctx, node.BMC, currentCreds, newUsername, newPassword, slot); err == nil {
		slog.InfoContext(ctx, "bmc hardware credentials synchronized via redfish", "node_id", nodeID)
		return nil
	} else {
		slog.WarnContext(ctx, "redfish credential update failed; attempting ipmi fallback", "node_id", nodeID, "error", err)
	}

	// Fallback to IPMI 2.0 RMCP+
	if err := m.ipmi.UpdateCredentials(ctx, node.BMC, currentCreds, newUsername, newPassword, slot); err != nil {
		return fmt.Errorf("failed synchronizing bmc credentials via redfish and ipmi: %w", err)
	}

	slog.InfoContext(ctx, "bmc hardware credentials synchronized via ipmi rmcp+", "node_id", nodeID)
	return nil
}
