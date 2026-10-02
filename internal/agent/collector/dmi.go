package collector

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// DMIInfo holds cataloged system information from SMBIOS/DMI.
type DMIInfo struct {
	Vendor       domain.Vendor
	Model        string
	SerialNumber string
	BIOSVersion  string
	FirmwareMode domain.FirmwareMode
}

// CollectDMI inspects system DMI tables and UEFI status.
func CollectDMI(ctx context.Context) (*DMIInfo, error) {
	info := &DMIInfo{
		Vendor:       domain.VendorGeneric,
		Model:        "Generic x86_64 Server",
		SerialNumber: "UNKNOWN",
		BIOSVersion:  "UNKNOWN",
		FirmwareMode: domain.FirmwareBIOS,
	}

	// Detect UEFI vs BIOS mode
	if _, err := os.Stat("/sys/firmware/efi"); err == nil {
		info.FirmwareMode = domain.FirmwareUEFI
	}

	// Read directly from sysfs /sys/class/dmi/id/
	vendorRaw := readDMIFile("/sys/class/dmi/id/sys_vendor")
	productRaw := readDMIFile("/sys/class/dmi/id/product_name")
	serialRaw := readDMIFile("/sys/class/dmi/id/product_serial")
	biosRaw := readDMIFile("/sys/class/dmi/id/bios_version")

	// If sysfs did not populate (e.g. some VM hypervisors), fall back to dmidecode
	if vendorRaw == "" {
		vendorRaw = runDMIDecode(ctx, "-s", "system-manufacturer")
	}
	if productRaw == "" {
		productRaw = runDMIDecode(ctx, "-s", "system-product-name")
	}
	if serialRaw == "" {
		serialRaw = runDMIDecode(ctx, "-s", "system-serial-number")
	}
	if biosRaw == "" {
		biosRaw = runDMIDecode(ctx, "-s", "bios-version")
	}

	if vendorRaw != "" {
		vLower := strings.ToLower(vendorRaw)
		switch {
		case strings.Contains(vLower, "dell"):
			info.Vendor = domain.VendorDell
		case strings.Contains(vLower, "supermicro"):
			info.Vendor = domain.VendorSupermicro
		case strings.Contains(vLower, "asrock"):
			info.Vendor = domain.VendorASRockRack
		default:
			info.Vendor = domain.VendorGeneric
		}
	}

	if productRaw != "" {
		info.Model = productRaw
	}
	if serialRaw != "" {
		info.SerialNumber = serialRaw
	}
	if biosRaw != "" {
		info.BIOSVersion = biosRaw
	}

	slog.InfoContext(ctx, "dmi telemetry collected",
		"vendor", info.Vendor,
		"model", info.Model,
		"serial", info.SerialNumber,
		"firmware", info.FirmwareMode,
	)

	return info, nil
}

func readDMIFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func runDMIDecode(ctx context.Context, args ...string) string {
	cmd := exec.CommandContext(ctx, "dmidecode", args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
