package collector

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// CollectBMC discovers, configures, and verifies out-of-band BMC management controllers.
func CollectBMC(ctx context.Context, vendor domain.Vendor) (*domain.BMCInfo, error) {
	bmc := &domain.BMCInfo{
		Vendor:             string(vendor),
		IP:                 "0.0.0.0",
		MAC:                "",
		DHCP:               true,
		Channel:            1,
		PortMode:           domain.PortModeDedicated,
		CredentialsUpdated: false,
	}

	// Verify IPMI KCS interface presence (/dev/ipmi0 or /dev/ipmidev/0)
	if !hasIPMIDevice() {
		slog.WarnContext(ctx, "no IPMI KCS character device found (/dev/ipmi0); skipping BMC discovery")
		return bmc, nil
	}

	// Step 1: Detect working LAN channel (usually 1, occasionally 2 or 8)
	channel := detectLANChannel(ctx)
	bmc.Channel = channel

	// Step 2: Query initial LAN parameters
	mac, ip, isDHCP, linkDetected := queryLANConfig(ctx, channel)
	bmc.MAC = mac
	bmc.IP = ip
	bmc.DHCP = isDHCP

	// Step 3: Multi-vendor specific port configuration (ONLY if unassigned or link down)
	// Adheres to non-destructive telemetry rule: do NOT disrupt operational BMC controllers or wipe static IPs
	if bmc.IP == "" || bmc.IP == "0.0.0.0" || !linkDetected {
		slog.InfoContext(ctx, "bmc network unconfigured or link down; configuring dedicated management port mode",
			"vendor", vendor,
			"channel", channel,
			"current_ip", bmc.IP,
			"link_detected", linkDetected,
		)
		configureDedicatedPort(ctx, vendor, channel)
	}

	// Step 4: STP / RSTP Polling Backoff
	// If BMC has no IP or 0.0.0.0, wait with backoff loop (up to 90 seconds)
	if bmc.IP == "" || bmc.IP == "0.0.0.0" {
		slog.InfoContext(ctx, "polling BMC IP address with STP/RSTP backoff loop",
			"vendor", vendor,
			"channel", channel,
			"timeout", "90s",
		)
		validIP := pollBMCIpWithBackoff(ctx, channel, 90*time.Second)
		if validIP != "" {
			bmc.IP = validIP
		}
	}

	slog.InfoContext(ctx, "bmc telemetry collected",
		"vendor", bmc.Vendor,
		"ip", bmc.IP,
		"mac", bmc.MAC,
		"dhcp", bmc.DHCP,
		"channel", bmc.Channel,
		"port_mode", bmc.PortMode,
	)

	return bmc, nil
}

func hasIPMIDevice() bool {
	paths := []string{"/dev/ipmi0", "/dev/ipmidev/0", "/dev/ipmi/0"}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func detectLANChannel(ctx context.Context) int {
	channels := []int{1, 2, 8}
	for _, ch := range channels {
		cmd := exec.CommandContext(ctx, "ipmitool", "lan", "print", fmt.Sprintf("%d", ch))
		out, err := cmd.Output()
		if err == nil && strings.Contains(string(out), "IP Address") {
			return ch
		}
	}
	return 1
}

func queryLANConfig(ctx context.Context, channel int) (mac string, ip string, isDHCP bool, linkDetected bool) {
	cmd := exec.CommandContext(ctx, "ipmitool", "lan", "print", fmt.Sprintf("%d", channel))
	out, err := cmd.Output()
	if err != nil {
		return "", "0.0.0.0", true, false
	}
	return parseLANPrintOutput(out)
}

func parseLANPrintOutput(out []byte) (mac string, ip string, isDHCP bool, linkDetected bool) {
	scanner := bufio.NewScanner(bytes.NewReader(out))
	isDHCP = true
	linkDetected = false
	ip = "0.0.0.0"
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		keyLower := strings.ToLower(key)
		valLower := strings.ToLower(val)

		switch {
		case strings.EqualFold(key, "IP Address") && !strings.Contains(key, "Source"):
			parsed := net.ParseIP(val)
			if parsed != nil {
				ip = parsed.String()
			}
		case strings.EqualFold(key, "MAC Address"):
			mac = strings.ToLower(val)
		case strings.EqualFold(key, "IP Address Source"):
			if strings.Contains(valLower, "static") {
				isDHCP = false
			}
		case strings.Contains(keyLower, "link status") || strings.Contains(keyLower, "link"):
			if strings.Contains(valLower, "detected") || strings.Contains(valLower, "up") || strings.Contains(valLower, "ok") {
				linkDetected = true
			}
		}
	}
	return mac, ip, isDHCP, linkDetected
}

func configureDedicatedPort(ctx context.Context, vendor domain.Vendor, channel int) {
	switch vendor {
	case domain.VendorSupermicro:
		// Supermicro AMI MegaRAC raw IPMI command for dedicated port mode:
		// NetFn: 0x30, Cmd: 0x70, SubCmd: 0x0c, Mode: 1 (Dedicated), Port: 0
		slog.InfoContext(ctx, "configuring Supermicro BMC dedicated port mode")
		cmd := exec.CommandContext(ctx, "ipmitool", "raw", "0x30", "0x70", "0x0c", "1", "0")
		_ = cmd.Run()

		// Ensure DHCP is enabled on BMC interface
		dhcpCmd := exec.CommandContext(ctx, "ipmitool", "lan", "set", fmt.Sprintf("%d", channel), "ipsrc", "dhcp")
		_ = dhcpCmd.Run()

	case domain.VendorASRockRack:
		slog.InfoContext(ctx, "ensuring ASRock Rack BMC DHCP source")
		dhcpCmd := exec.CommandContext(ctx, "ipmitool", "lan", "set", fmt.Sprintf("%d", channel), "ipsrc", "dhcp")
		_ = dhcpCmd.Run()

	case domain.VendorDell:
		slog.InfoContext(ctx, "verifying Dell iDRAC DHCP source")
		dhcpCmd := exec.CommandContext(ctx, "ipmitool", "lan", "set", fmt.Sprintf("%d", channel), "ipsrc", "dhcp")
		_ = dhcpCmd.Run()
	}
}

func pollBMCIpWithBackoff(ctx context.Context, channel int, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ""
		case now := <-ticker.C:
			if now.After(deadline) {
				return ""
			}

			_, ip, _, linkDetected := queryLANConfig(ctx, channel)
			if !linkDetected {
				slog.DebugContext(ctx, "bmc switch port stp negotiation in progress (link carrier not yet detected)", "channel", channel)
			}
			if ip != "" && ip != "0.0.0.0" {
				parsed := net.ParseIP(ip)
				if parsed != nil && !parsed.IsUnspecified() && !parsed.IsLoopback() {
					slog.InfoContext(ctx, "bmc ip successfully resolved via dhcp", "ip", ip, "link_detected", linkDetected)
					return ip
				}
			}
		}
	}
}
