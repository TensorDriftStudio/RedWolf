package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

func TestImageCatalog_ListImages_AllSupportedTargets(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	catalog := NewImageCatalogService(tmpDir)
	images, err := catalog.ListImages(ctx)
	if err != nil {
		t.Fatalf("unexpected error listing images: %v", err)
	}

	if len(images) != 5 {
		t.Fatalf("expected 5 supported targets, got %d", len(images))
	}

	expectedOS := map[domain.OperatingSystem]bool{
		domain.OSAlmaLinux9:  false,
		domain.OSDebian12:    false,
		domain.OSAlmaLinux8:  false,
		domain.OSAlmaLinux10: false,
		domain.OSDebian13:    false,
	}

	for _, img := range images {
		if _, exists := expectedOS[img.OS]; !exists {
			t.Errorf("unexpected OS in catalog: %s", img.OS)
		}
		expectedOS[img.OS] = true

		if img.DownloadURL == "" {
			t.Errorf("expected OS %s to have configured DownloadURL, but got empty string", img.OS)
		}

		if img.OS == domain.OSAlmaLinux10 && img.DisplayName != "AlmaLinux 10 (Enterprise LTS)" {
			t.Errorf("expected AlmaLinux 10 display name to be 'AlmaLinux 10 (Enterprise LTS)', got '%s'", img.DisplayName)
		}

		if img.OS == domain.OSDebian13 && img.DisplayName != "Debian 13 Trixie (Stable LTS)" {
			t.Errorf("expected Debian 13 display name to be 'Debian 13 Trixie (Stable LTS)', got '%s'", img.DisplayName)
		}

		if img.Present {
			t.Errorf("expected image %s to not be present initially in empty tmpDir", img.OS)
		}
	}

	for osType, found := range expectedOS {
		if !found {
			t.Errorf("missing expected OS target: %s", osType)
		}
	}
}

func TestImageCatalog_PresenceDetection(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Create dummy image file for Debian 13
	dummyDebian13 := filepath.Join(tmpDir, "debian-13-genericcloud-amd64.raw")
	if err := os.WriteFile(dummyDebian13, []byte("DEBIAN_RAW_DATA"), 0644); err != nil {
		t.Fatalf("failed creating test image: %v", err)
	}

	catalog := NewImageCatalogService(tmpDir)
	images, err := catalog.ListImages(ctx)
	if err != nil {
		t.Fatalf("unexpected error listing images: %v", err)
	}

	foundDebian13 := false
	for _, img := range images {
		if img.OS == domain.OSDebian13 {
			foundDebian13 = true
			if !img.Present {
				t.Fatalf("expected Debian 13 to be detected as present")
			}
			if img.Filename != "debian-13-genericcloud-amd64.raw" {
				t.Fatalf("expected Debian 13 filename to match created dummy file, got %s", img.Filename)
			}
			if img.SizeBytes != int64(len("DEBIAN_RAW_DATA")) {
				t.Fatalf("expected Debian 13 size to match created dummy file, got %d", img.SizeBytes)
			}
		}
	}

	if !foundDebian13 {
		t.Fatalf("Debian 13 not found in image catalog")
	}

	filename, err := catalog.GetImageFilename(ctx, domain.OSDebian13)
	if err != nil {
		t.Fatalf("failed resolving filename: %v", err)
	}
	if filename != "debian-13-genericcloud-amd64.raw" {
		t.Fatalf("expected resolved filename to be debian-13-genericcloud-amd64.raw, got %s", filename)
	}

	if !catalog.IsImagePresent(ctx, domain.OSDebian13) {
		t.Fatalf("expected IsImagePresent to return true for Debian 13")
	}
	if catalog.IsImagePresent(ctx, domain.OSAlmaLinux8) {
		t.Fatalf("expected IsImagePresent to return false for un-cached AlmaLinux 8")
	}
}

func TestImageCatalog_GetOSSlug(t *testing.T) {
	tests := []struct {
		os       domain.OperatingSystem
		expected string
	}{
		{domain.OSAlmaLinux8, "almalinux-8"},
		{domain.OSAlmaLinux9, "almalinux-9"},
		{domain.OSAlmaLinux10, "almalinux-10"},
		{domain.OSDebian12, "debian-12"},
		{domain.OSDebian13, "debian-13"},
	}

	for _, tc := range tests {
		slug := getOSSlug(tc.os)
		if slug != tc.expected {
			t.Errorf("for OS %s expected slug %s, got %s", tc.os, tc.expected, slug)
		}
	}
}
