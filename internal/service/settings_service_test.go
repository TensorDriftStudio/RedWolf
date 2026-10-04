package service

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSettingsService_SubnetSanitization(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed opening in-memory db: %v", err)
	}
	defer db.Close()

	svc := NewSettingsService(db)
	ctx := context.Background()

	// Initial default should be 192.168.0.0/24
	settings, err := svc.GetSettings(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings.Network.SubnetCIDR != "192.168.0.0/24" {
		t.Errorf("expected default 192.168.0.0/24, got %s", settings.Network.SubnetCIDR)
	}

	// Update settings with gateway on 192.168.50.1 but leaving default subnet CIDR
	updated := *settings
	updated.Network.Gateway = "192.168.50.1"
	updated.Network.DHCPRangeStart = "192.168.50.100"
	updated.Network.DHCPRangeEnd = "192.168.50.200"

	if err := svc.UpdateSettings(ctx, updated); err != nil {
		t.Fatalf("failed updating settings: %v", err)
	}

	saved, err := svc.GetSettings(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved.Network.SubnetCIDR != "192.168.50.0/24" {
		t.Errorf("expected auto-sanitized 192.168.50.0/24, got %s", saved.Network.SubnetCIDR)
	}

	// Reload from DB to verify persistence of sanitized CIDR
	reloadedSvc := NewSettingsService(db)
	persisted, err := reloadedSvc.GetSettings(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if persisted.Network.SubnetCIDR != "192.168.50.0/24" {
		t.Errorf("expected persisted 192.168.50.0/24, got %s", persisted.Network.SubnetCIDR)
	}
}
