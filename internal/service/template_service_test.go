package service_test

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/service"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed opening in-memory db: %v", err)
	}
	return db
}

func TestTemplateService_Lifecycle(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	svc := service.NewTemplateService(db)
	ctx := context.Background()

	// 1. Verify default templates were seeded
	templates, err := svc.ListTemplates(ctx)
	if err != nil {
		t.Fatalf("failed listing templates: %v", err)
	}
	if len(templates) < 3 {
		t.Fatalf("expected at least 3 seeded templates, got %d", len(templates))
	}

	// 2. Create custom template
	custom := &domain.CloudInitTemplate{
		Name:        "Ceph Storage Node",
		Description: "Configures kernel modules and tuned sysctl for distributed Ceph OSD nodes",
		Distro:      "AlmaLinux 9",
		UserData: `#cloud-config
growpart:
  mode: auto
package_update: true
packages:
  - lvm2
  - xfsprogs
`,
		IsDefault: false,
	}

	if err := svc.CreateTemplate(ctx, custom); err != nil {
		t.Fatalf("failed creating template: %v", err)
	}

	if custom.ID == "" {
		t.Fatal("expected generated template ID, got empty")
	}

	// 3. Retrieve template by ID
	retrieved, err := svc.GetTemplate(ctx, custom.ID)
	if err != nil {
		t.Fatalf("failed getting template %s: %v", custom.ID, err)
	}
	if retrieved.Name != custom.Name {
		t.Errorf("expected name %q, got %q", custom.Name, retrieved.Name)
	}

	// 4. Update template
	retrieved.Description = "Updated Ceph description"
	if err := svc.UpdateTemplate(ctx, custom.ID, retrieved); err != nil {
		t.Fatalf("failed updating template: %v", err)
	}

	updated, _ := svc.GetTemplate(ctx, custom.ID)
	if updated.Description != "Updated Ceph description" {
		t.Errorf("expected updated description, got %q", updated.Description)
	}

	// 5. Delete template
	if err := svc.DeleteTemplate(ctx, custom.ID); err != nil {
		t.Fatalf("failed deleting template: %v", err)
	}

	_, err = svc.GetTemplate(ctx, custom.ID)
	if err == nil {
		t.Fatal("expected error after deletion, got nil")
	}
}
