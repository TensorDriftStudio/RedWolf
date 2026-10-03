package collector

import (
	"bufio"
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// CollectNetwork inspects physical network interfaces, link states, and boot routes.
func CollectNetwork(ctx context.Context) ([]domain.NetworkInterface, error) {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return nil, err
	}

	bootIface := detectBootInterface(ctx)

	var interfaces []domain.NetworkInterface
	for _, entry := range entries {
		name := entry.Name()

		// Skip loopback, virtual switches, docker, and veth devices
		if name == "lo" || strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "veth") ||
			strings.HasPrefix(name, "br-") || strings.HasPrefix(name, "virbr") {
			continue
		}

		mac := readSysfsString(filepath.Join("/sys/class/net", name, "address"))
		if mac == "" || mac == "00:00:00:00:00:00" {
			continue
		}
		mac = strings.ToLower(mac)

		carrier := readSysfsInt(filepath.Join("/sys/class/net", name, "carrier")) == 1
		speed := readSysfsInt(filepath.Join("/sys/class/net", name, "speed"))
		if speed <= 0 {
			speed = 1000 // Default 1 GbE if unnegotiated
		}

		driver := detectDriver(ctx, name)
		pciSlot := detectPCISlot(name)

		isBoot := (name == bootIface)

		nic := domain.NetworkInterface{
			Name:      name,
			MAC:       mac,
			SpeedMbps: speed,
			Carrier:   carrier,
			IsBoot:    isBoot,
			Driver:    driver,
			PCISlot:   pciSlot,
		}

		interfaces = append(interfaces, nic)
		slog.InfoContext(ctx, "network interface cataloged",
			"name", nic.Name,
			"mac", nic.MAC,
			"speed_mbps", nic.SpeedMbps,
			"carrier", nic.Carrier,
			"is_boot", nic.IsBoot,
			"driver", nic.Driver,
		)
	}

	// If no interface was explicitly marked as boot, mark the first carrier-active interface
	hasBoot := false
	for _, nic := range interfaces {
		if nic.IsBoot {
			hasBoot = true
			break
		}
	}
	if !hasBoot && len(interfaces) > 0 {
		for i := range interfaces {
			if interfaces[i].Carrier {
				interfaces[i].IsBoot = true
				hasBoot = true
				break
			}
		}
		if !hasBoot {
			interfaces[0].IsBoot = true
		}
	}

	return interfaces, nil
}

func detectBootInterface(ctx context.Context) string {
	// Parse default route from /proc/net/route
	file, err := os.Open("/proc/net/route")
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			// Destination 00000000 means default gateway
			if len(fields) >= 2 && fields[1] == "00000000" {
				return fields[0]
			}
		}
		if err := scanner.Err(); err != nil {
			slog.DebugContext(ctx, "scanner error reading /proc/net/route", "error", err)
		}
	}

	// Fallback to ip route
	cmd := exec.CommandContext(ctx, "ip", "route", "show", "default")
	out, err := cmd.Output()
	if err == nil {
		fields := strings.Fields(string(out))
		for i, f := range fields {
			if f == "dev" && i+1 < len(fields) {
				return fields[i+1]
			}
		}
	}
	return ""
}

func detectDriver(ctx context.Context, iface string) string {
	// Try sysfs driver symlink
	driverPath := filepath.Join("/sys/class/net", iface, "device", "driver")
	target, err := filepath.EvalSymlinks(driverPath)
	if err == nil {
		return filepath.Base(target)
	}

	// Try ethtool -i
	cmd := exec.CommandContext(ctx, "ethtool", "-i", iface)
	out, err := cmd.Output()
	if err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(out)))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "driver:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					return strings.TrimSpace(parts[1])
				}
			}
		}
	}
	return "generic"
}

func detectPCISlot(iface string) string {
	devPath := filepath.Join("/sys/class/net", iface, "device")
	target, err := filepath.EvalSymlinks(devPath)
	if err == nil {
		return filepath.Base(target)
	}
	return ""
}

func readSysfsString(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readSysfsInt(path string) int {
	str := readSysfsString(path)
	if str == "" {
		return -1
	}
	val, err := strconv.Atoi(str)
	if err != nil {
		return -1
	}
	return val
}
