package dnsmasq

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderConfig_ProxyDHCP(t *testing.T) {
	tempDir := t.TempDir()
	confDir := filepath.Join(tempDir, "conf")
	tftpDir := filepath.Join(tempDir, "tftp")
	logDir := filepath.Join(tempDir, "log")

	mgr := NewManager(Config{
		Interface: "eth0",
		ServerIP:  "192.168.1.50",
		HTTPPort:  8080,
		ConfDir:   confDir,
		TFTPDir:   tftpDir,
		LogDir:    logDir,
		ProxyDHCP: true,
	})

	ctx := context.Background()
	if err := mgr.RenderConfig(ctx); err != nil {
		t.Fatalf("RenderConfig failed: %v", err)
	}

	confFile := filepath.Join(confDir, "dnsmasq.conf")
	content, err := os.ReadFile(confFile)
	if err != nil {
		t.Fatalf("failed to read rendered config: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "dhcp-range=192.168.1.50,proxy") {
		t.Errorf("expected proxy dhcp-range in config, got:\n%s", s)
	}
	if !strings.Contains(s, "dhcp-boot=tag:!ipxe,tag:efi64,ipxe.efi") {
		t.Errorf("expected efi64 dhcp-boot in config, got:\n%s", s)
	}
	// ipxe-arm64.efi should be disabled since it does not exist in tftpDir
	if !strings.Contains(s, "# dhcp-boot=tag:!ipxe,tag:arm64,ipxe-arm64.efi") {
		t.Errorf("expected commented out arm64 dhcp-boot when binary absent, got:\n%s", s)
	}
}

func TestRenderConfig_WithArm64Present(t *testing.T) {
	tempDir := t.TempDir()
	confDir := filepath.Join(tempDir, "conf")
	tftpDir := filepath.Join(tempDir, "tftp")
	logDir := filepath.Join(tempDir, "log")

	_ = os.MkdirAll(tftpDir, 0755)
	_ = os.WriteFile(filepath.Join(tftpDir, "ipxe-arm64.efi"), []byte("mock-arm64"), 0644)

	mgr := NewManager(Config{
		Interface: "eth0",
		ServerIP:  "10.0.0.1",
		HTTPPort:  8080,
		ConfDir:   confDir,
		TFTPDir:   tftpDir,
		LogDir:    logDir,
		ProxyDHCP: true,
	})

	ctx := context.Background()
	if err := mgr.RenderConfig(ctx); err != nil {
		t.Fatalf("RenderConfig failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(confDir, "dnsmasq.conf"))
	if err != nil {
		t.Fatalf("failed to read rendered config: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "dhcp-boot=tag:!ipxe,tag:arm64,ipxe-arm64.efi\n") {
		t.Errorf("expected active arm64 dhcp-boot when binary present, got:\n%s", s)
	}
}
