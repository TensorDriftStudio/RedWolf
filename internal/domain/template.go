package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrTemplateNotFound = errors.New("cloud-init template not found")
	ErrInvalidTemplate  = errors.New("invalid cloud-init template")
)

// CloudInitTemplate represents a reusable cloud-init user-data configuration template.
type CloudInitTemplate struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Distro      string    `json:"distro"` // e.g. "All", "AlmaLinux", "Debian"
	UserData    string    `json:"userData"` // Raw cloud-config YAML
	IsDefault   bool      `json:"isDefault"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Validate checks that required fields in the template are present and valid.
func (t *CloudInitTemplate) Validate() error {
	if strings.TrimSpace(t.Name) == "" {
		return errors.New("template name is required")
	}
	if strings.TrimSpace(t.UserData) == "" {
		return errors.New("template userData YAML is required")
	}
	if !strings.HasPrefix(strings.TrimSpace(t.UserData), "#cloud-config") {
		return errors.New("template userData must begin with '#cloud-config'")
	}
	return nil
}
