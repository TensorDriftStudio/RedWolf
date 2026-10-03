package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/port"
)

// Provisioner coordinates node lifecycle, hardware inventory registration, and deployment tasks.
type Provisioner struct {
	repo         port.NodeRepository
	events       port.EventBroadcaster
	templates    *TemplateService
	imageCatalog *ImageCatalogService
	tasks        sync.Map // Map[string]*domain.DeploymentTask keyed by nodeID
}

// NewProvisioner creates an initialized service coordinator.
func NewProvisioner(repo port.NodeRepository, events port.EventBroadcaster) *Provisioner {
	return &Provisioner{
		repo:   repo,
		events: events,
	}
}

// SetTemplateService associates the Cloud-Init template service with the provisioner.
func (p *Provisioner) SetTemplateService(ts *TemplateService) {
	p.templates = ts
}

// SetImageCatalog associates the OS image catalog with the provisioner for pre-flight validation.
func (p *Provisioner) SetImageCatalog(ic *ImageCatalogService) {
	p.imageCatalog = ic
}

// RegisterDiscoveredNode ingests machine-readable telemetry from an in-memory discovery agent.
func (p *Provisioner) RegisterDiscoveredNode(ctx context.Context, node *domain.ServerNode) (*domain.ServerNode, error) {
	if node.ID == "" {
		node.ID = uuid.NewString()
	}

	// Verify if node exists by serial number or MAC
	var existing *domain.ServerNode
	var err error
	if bootMAC := node.BootMAC(); bootMAC != "" {
		existing, err = p.repo.GetByMAC(ctx, bootMAC)
		if err != nil && !domain.ErrNodeNotFoundIs(err) {
			slog.WarnContext(ctx, "error querying node by mac", "mac", bootMAC, "error", err)
		}
	}

	if existing != nil {
		node.ID = existing.ID
		node.DiscoveredAt = existing.DiscoveredAt
		if existing.Status == domain.NodeStatusActive {
			// Keep active state unless explicitly overridden
			node.Status = domain.NodeStatusActive
		} else {
			node.Status = domain.NodeStatusReady
		}
	} else {
		node.Status = domain.NodeStatusReady
		node.DiscoveredAt = time.Now().UTC()
	}

	node.UpdatedAt = time.Now().UTC()

	if err := p.repo.Save(ctx, node); err != nil {
		return nil, fmt.Errorf("failed to persist discovered node: %w", err)
	}

	slog.InfoContext(ctx, "registered discovered node",
		"node_id", node.ID,
		"vendor", node.Vendor,
		"model", node.Model,
		"serial", node.SerialNumber,
		"mac", node.BootMAC(),
	)

	p.events.Publish("node:updated", node)
	return node, nil
}

// GetNode retrieves a server node by ID.
func (p *Provisioner) GetNode(ctx context.Context, id string) (*domain.ServerNode, error) {
	return p.repo.GetByID(ctx, id)
}

// GetNodeByMAC looks up a node by any associated MAC address.
func (p *Provisioner) GetNodeByMAC(ctx context.Context, mac string) (*domain.ServerNode, error) {
	return p.repo.GetByMAC(ctx, mac)
}

// ListNodes retrieves all managed nodes.
func (p *Provisioner) ListNodes(ctx context.Context) ([]*domain.ServerNode, error) {
	return p.repo.List(ctx)
}

// InitiateDeployment validates and launches bare-metal OS provisioning for a node.
func (p *Provisioner) InitiateDeployment(ctx context.Context, cfg domain.DeploymentConfig) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid deployment configuration: %w", err)
	}

	// Verify image availability if ImageCatalog is attached
	var imageFilename string
	if p.imageCatalog != nil {
		if !p.imageCatalog.IsImagePresent(ctx, cfg.OS) {
			return fmt.Errorf("operating system image for %s is not cached on appliance; please download or place it in data/images first", cfg.OS)
		}
		var err error
		imageFilename, err = p.imageCatalog.GetImageFilename(ctx, cfg.OS)
		if err != nil {
			return fmt.Errorf("failed to resolve image filename for %s: %w", cfg.OS, err)
		}
	} else {
		// Fallback default filenames when catalog service is unconfigured (e.g. testing)
		switch cfg.OS {
		case domain.OSAlmaLinux9:
			imageFilename = "almalinux-9-genericcloud.raw.zstd"
		case domain.OSAlmaLinux8:
			imageFilename = "almalinux-8-genericcloud.raw.zstd"
		case domain.OSDebian12:
			imageFilename = "debian-12-genericcloud.raw.zstd"
		case domain.OSAlmaLinux10:
			imageFilename = "almalinux-10-genericcloud.raw.zstd"
		case domain.OSDebian13:
			imageFilename = "debian-13-genericcloud.raw.zstd"
		default:
			imageFilename = fmt.Sprintf("%s-genericcloud.raw.zstd", cfg.OS)
		}
	}

	// Resolve Cloud-Init template if specified
	if cfg.TemplateID != "" && cfg.CustomUserData == "" && p.templates != nil {
		if tpl, err := p.templates.GetTemplate(ctx, cfg.TemplateID); err == nil {
			cfg.CustomUserData = tpl.UserData
			slog.InfoContext(ctx, "applied cloud-init template to deployment", "node_id", cfg.NodeID, "template_id", cfg.TemplateID, "template_name", tpl.Name)
		} else {
			slog.WarnContext(ctx, "referenced template could not be loaded; using default configuration", "template_id", cfg.TemplateID, "error", err)
		}
	}

	node, err := p.repo.GetByID(ctx, cfg.NodeID)
	if err != nil {
		return fmt.Errorf("node %s not found: %w", cfg.NodeID, err)
	}

	if err := node.TransitionTo(domain.NodeStatusProvisioning); err != nil {
		return err
	}

	targetIP := "DHCP"
	if cfg.NetworkMode == domain.NetworkModeStatic {
		targetIP = cfg.StaticIP
	}

	initialState := &domain.ProvisioningState{
		Progress:    5,
		Stage:       fmt.Sprintf("Initiating %s deployment to %s...", cfg.OS, cfg.TargetDrivePath),
		OS:          string(cfg.OS),
		TargetDrive: cfg.TargetDrivePath,
		TargetIP:    targetIP,
		Logs: []string{
			fmt.Sprintf("[%s] Provisioning session authorized by operator", time.Now().UTC().Format("15:04:05")),
			fmt.Sprintf("[%s] Target disk selected: %s", time.Now().UTC().Format("15:04:05"), cfg.TargetDrivePath),
			fmt.Sprintf("[%s] Distribution image queued: %s (%s)", time.Now().UTC().Format("15:04:05"), cfg.OS, imageFilename),
		},
	}
	node.ProvisioningState = initialState

	if err := p.repo.Save(ctx, node); err != nil {
		return fmt.Errorf("failed to update node to provisioning state: %w", err)
	}

	task := &domain.DeploymentTask{
		TaskID:          uuid.NewString(),
		NodeID:          node.ID,
		OS:              cfg.OS,
		ImageURL:        fmt.Sprintf("/assets/images/%s", imageFilename),
		TargetDrivePath: cfg.TargetDrivePath,
		Config:          cfg,
	}
	p.tasks.Store(node.ID, task)

	slog.InfoContext(ctx, "bare-metal deployment initiated",
		"node_id", node.ID,
		"os", cfg.OS,
		"target_drive", cfg.TargetDrivePath,
		"ip", targetIP,
	)

	p.events.Publish("node:updated", node)
	return nil
}

// GetPendingTask retrieves the active provisioning task for a node.
func (p *Provisioner) GetPendingTask(ctx context.Context, nodeID string) (*domain.DeploymentTask, error) {
	val, ok := p.tasks.Load(nodeID)
	if !ok {
		return nil, domain.ErrNodeNotFound
	}
	return val.(*domain.DeploymentTask), nil
}

// GetPendingTaskByMAC looks up the active provisioning task for a node by its boot MAC.
func (p *Provisioner) GetPendingTaskByMAC(ctx context.Context, mac string) (*domain.DeploymentTask, error) {
	node, err := p.repo.GetByMAC(ctx, mac)
	if err != nil {
		return nil, err
	}
	return p.GetPendingTask(ctx, node.ID)
}

// UpdateProgress updates the live deployment progress and log stream for a node.
func (p *Provisioner) UpdateProgress(ctx context.Context, nodeID string, progress int, stage, logMsg string) error {
	node, err := p.repo.GetByID(ctx, nodeID)
	if err != nil {
		return err
	}

	if node.ProvisioningState == nil {
		node.ProvisioningState = &domain.ProvisioningState{}
	}

	node.ProvisioningState.Progress = progress
	if stage != "" {
		node.ProvisioningState.Stage = stage
	}
	if logMsg != "" {
		timestamped := fmt.Sprintf("[%s] %s", time.Now().UTC().Format("15:04:05"), logMsg)
		node.ProvisioningState.Logs = append(node.ProvisioningState.Logs, timestamped)
	}

	if progress >= 100 {
		_ = node.TransitionTo(domain.NodeStatusActive)
		node.ProvisioningState.Stage = "Active in Production"
		p.tasks.Delete(nodeID)
	} else if strings.Contains(strings.ToLower(stage), "failed") || strings.Contains(strings.ToLower(stage), "error") || progress < 0 {
		_ = node.TransitionTo(domain.NodeStatusError)
		p.tasks.Delete(nodeID)
	}

	if err := p.repo.Save(ctx, node); err != nil {
		return err
	}

	p.events.Publish("node:updated", node)
	return nil
}

// ResetNode transitions an ACTIVE or ERROR node back to READY_FOR_PROVISIONING and clears ongoing provisioning state.
func (p *Provisioner) ResetNode(ctx context.Context, nodeID string) (*domain.ServerNode, error) {
	node, err := p.repo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, err
	}

	p.tasks.Delete(nodeID)
	node.Status = domain.NodeStatusReady
	node.ProvisioningState = nil
	node.UpdatedAt = time.Now().UTC()

	if err := p.repo.Save(ctx, node); err != nil {
		return nil, fmt.Errorf("failed saving reset node: %w", err)
	}

	slog.InfoContext(ctx, "reset node status to ready for provisioning", "node_id", nodeID)
	p.events.Publish("node:updated", node)
	return node, nil
}

// DeleteNode removes a node and all its records from the repository.
func (p *Provisioner) DeleteNode(ctx context.Context, nodeID string) error {
	p.tasks.Delete(nodeID)
	if err := p.repo.Delete(ctx, nodeID); err != nil {
		return fmt.Errorf("failed deleting node %s: %w", nodeID, err)
	}

	slog.InfoContext(ctx, "deleted node from inventory", "node_id", nodeID)
	p.events.Publish("node:deleted", map[string]string{"id": nodeID})
	return nil
}
