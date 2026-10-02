package provision

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// ConfigureBootloader registers the UEFI bootloader in system NVRAM via efibootmgr.
func ConfigureBootloader(ctx context.Context, targetDrivePath string, osType domain.OperatingSystem) error {
	slog.InfoContext(ctx, "configuring UEFI bootloader in NVRAM",
		"target_drive", targetDrivePath,
		"os", osType,
	)

	// Determine EFI loader path based on distribution
	loaderPath := `\EFI\BOOT\BOOTX64.EFI`
	switch osType {
	case domain.OSAlmaLinux8, domain.OSAlmaLinux9, domain.OSAlmaLinux10:
		loaderPath = `\EFI\almalinux\shimx64.efi`
	case domain.OSDebian12, domain.OSDebian13:
		loaderPath = `\EFI\debian\shimx64.efi`
	}

	label := fmt.Sprintf("RedWolf (%s)", osType)

	// Register boot entry on partition 1 (standard EFI system partition)
	cmd := exec.CommandContext(ctx, "efibootmgr",
		"-c",
		"-d", targetDrivePath,
		"-p", "1",
		"-L", label,
		"-l", loaderPath,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.WarnContext(ctx, "efibootmgr returned warning/error (system might be in BIOS mode or virtualized)",
			"error", err,
			"output", string(out),
		)
		// Try fallback generic loader path
		fallbackCmd := exec.CommandContext(ctx, "efibootmgr",
			"-c",
			"-d", targetDrivePath,
			"-p", "1",
			"-L", label,
			"-l", `\EFI\BOOT\BOOTX64.EFI`,
		)
		_ = fallbackCmd.Run()
	} else {
		slog.InfoContext(ctx, "uefi bootloader registered successfully",
			"target_drive", targetDrivePath,
			"loader", loaderPath,
		)
	}

	return nil
}
