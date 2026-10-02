package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// TemplateService manages reusable Cloud-Init templates in SQLite.
type TemplateService struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewTemplateService initializes the template store and ensures default templates exist.
func NewTemplateService(db *sql.DB) *TemplateService {
	svc := &TemplateService{db: db}
	svc.initSchema()
	svc.seedDefaults()
	return svc
}

func (s *TemplateService) initSchema() {
	if s.db == nil {
		return
	}
	query := `CREATE TABLE IF NOT EXISTS cloud_init_templates (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT NOT NULL,
		distro TEXT NOT NULL,
		user_data TEXT NOT NULL,
		is_default INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_cloud_init_templates_default ON cloud_init_templates(is_default);`
	if _, err := s.db.Exec(query); err != nil {
		slog.Error("failed initializing cloud_init_templates schema", "error", err)
	}
}

func (s *TemplateService) seedDefaults() {
	if s.db == nil {
		return
	}
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM cloud_init_templates").Scan(&count)
	if err != nil || count > 0 {
		return
	}

	defaults := []domain.CloudInitTemplate{
		{
			ID:          "tpl-base-minimal",
			Name:        "Minimal Base Server",
			Description: "Standard production bare-metal configuration with automatic rootfs expansion and essential diagnostic utilities.",
			Distro:      "All",
			IsDefault:   true,
			UserData: `#cloud-config
growpart:
  mode: auto
  devices: ['/']
  ignore_growpart_interface: false
resize_rootfs: true

package_update: true
package_upgrade: false
packages:
  - curl
  - wget
  - htop
  - jq
  - tar
  - util-linux
  - ca-certificates

runcmd:
  - echo "RedWolf bare-metal node initialized successfully" > /etc/redwolf-release
`,
		},
		{
			ID:          "tpl-docker-host",
			Name:        "Container & Docker Engine Host",
			Description: "Prepares node with kernel container modules, system limits, and container runtime prerequisites.",
			Distro:      "All",
			IsDefault:   false,
			UserData: `#cloud-config
growpart:
  mode: auto
  devices: ['/']
  ignore_growpart_interface: false
resize_rootfs: true

package_update: true
packages:
  - curl
  - iptables
  - ca-certificates
  - gnupg

write_files:
  - path: /etc/sysctl.d/99-kubernetes-cri.conf
    permissions: '0644'
    content: |
      net.bridge.bridge-nf-call-iptables  = 1
      net.ipv4.ip_forward                 = 1
      net.bridge.bridge-nf-call-ip6tables = 1

runcmd:
  - modprobe overlay
  - modprobe br_netfilter
  - sysctl --system
`,
		},
		{
			ID:          "tpl-k8s-node",
			Name:        "Kubernetes Ready Node",
			Description: "Hardens node for K8s / Talos / K3s join: disables swap, tunes network buffers, and loads bridge modules.",
			Distro:      "All",
			IsDefault:   false,
			UserData: `#cloud-config
growpart:
  mode: auto
  devices: ['/']
  ignore_growpart_interface: false
resize_rootfs: true

# Disable swap for kubelet stability
swap:
  filename: ""
  size: "0"
  maxsize: "0"

package_update: true
packages:
  - curl
  - socat
  - conntrack
  - ipset
  - ebtables

runcmd:
  - swapoff -a
  - sed -i '/swap/d' /etc/fstab
  - echo "br_netfilter" >> /etc/modules-load.d/k8s.conf
  - echo "overlay" >> /etc/modules-load.d/k8s.conf
  - modprobe br_netfilter
  - modprobe overlay
`,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, t := range defaults {
		_ = s.CreateTemplate(ctx, &t)
	}
	slog.Info("seeded default cloud-init templates into database")
}

// ListTemplates retrieves all available Cloud-Init templates.
func (s *TemplateService) ListTemplates(ctx context.Context) ([]domain.CloudInitTemplate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, name, description, distro, user_data, is_default, created_at, updated_at
	          FROM cloud_init_templates ORDER BY is_default DESC, name ASC`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed querying templates: %w", err)
	}
	defer rows.Close()

	var templates []domain.CloudInitTemplate
	for rows.Next() {
		var t domain.CloudInitTemplate
		var isDef int
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Distro, &t.UserData, &isDef, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed scanning template row: %w", err)
		}
		t.IsDefault = isDef == 1
		templates = append(templates, t)
	}
	return templates, nil
}

// GetTemplate retrieves a template by ID.
func (s *TemplateService) GetTemplate(ctx context.Context, id string) (*domain.CloudInitTemplate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, name, description, distro, user_data, is_default, created_at, updated_at
	          FROM cloud_init_templates WHERE id = ?`
	var t domain.CloudInitTemplate
	var isDef int
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&t.ID, &t.Name, &t.Description, &t.Distro, &t.UserData, &isDef, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrTemplateNotFound
		}
		return nil, fmt.Errorf("failed querying template %s: %w", id, err)
	}
	t.IsDefault = isDef == 1
	return &t, nil
}

// CreateTemplate stores a new Cloud-Init template.
func (s *TemplateService) CreateTemplate(ctx context.Context, tpl *domain.CloudInitTemplate) error {
	if err := tpl.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if tpl.ID == "" {
		tpl.ID = fmt.Sprintf("tpl-%s", uuid.NewString()[:8])
	}
	now := time.Now().UTC()
	tpl.CreatedAt = now
	tpl.UpdatedAt = now

	// If this template is set as default, unset other defaults
	if tpl.IsDefault {
		_, _ = s.db.ExecContext(ctx, "UPDATE cloud_init_templates SET is_default = 0")
	}

	isDef := 0
	if tpl.IsDefault {
		isDef = 1
	}

	query := `INSERT INTO cloud_init_templates (id, name, description, distro, user_data, is_default, created_at, updated_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := s.db.ExecContext(ctx, query,
		tpl.ID, tpl.Name, tpl.Description, tpl.Distro, tpl.UserData, isDef, tpl.CreatedAt, tpl.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed inserting template: %w", err)
	}
	return nil
}

// UpdateTemplate updates an existing Cloud-Init template.
func (s *TemplateService) UpdateTemplate(ctx context.Context, id string, tpl *domain.CloudInitTemplate) error {
	if err := tpl.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if tpl.IsDefault {
		_, _ = s.db.ExecContext(ctx, "UPDATE cloud_init_templates SET is_default = 0 WHERE id != ?", id)
	}

	isDef := 0
	if tpl.IsDefault {
		isDef = 1
	}

	now := time.Now().UTC()
	query := `UPDATE cloud_init_templates 
	          SET name = ?, description = ?, distro = ?, user_data = ?, is_default = ?, updated_at = ?
	          WHERE id = ?`
	res, err := s.db.ExecContext(ctx, query,
		tpl.Name, tpl.Description, tpl.Distro, tpl.UserData, isDef, now, id,
	)
	if err != nil {
		return fmt.Errorf("failed updating template %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrTemplateNotFound
	}
	tpl.ID = id
	tpl.UpdatedAt = now
	return nil
}

// DeleteTemplate removes a Cloud-Init template.
func (s *TemplateService) DeleteTemplate(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `DELETE FROM cloud_init_templates WHERE id = ?`
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed deleting template %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrTemplateNotFound
	}
	return nil
}
