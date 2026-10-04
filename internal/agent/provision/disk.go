package provision

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
)

// WipeTargetDisk cleans existing partition tables, wipes filesystem signatures, and resets the target drive.
func WipeTargetDisk(ctx context.Context, targetDrive string) error {
	realDev, err := filepath.EvalSymlinks(targetDrive)
	if err != nil {
		realDev = targetDrive
	}

	slog.InfoContext(ctx, "pre-wiping target block storage device", "target_drive", targetDrive, "real_dev", realDev)

	// Step 1: Wipe filesystem and partition signatures
	wipeCmd := exec.CommandContext(ctx, "wipefs", "-a", "-f", realDev)
	_ = wipeCmd.Run()

	// Step 2: Attempt hardware block discard (TRIM for SSD/NVMe)
	discardCmd := exec.CommandContext(ctx, "blkdiscard", realDev)
	if err := discardCmd.Run(); err == nil {
		slog.InfoContext(ctx, "hardware blkdiscard completed successfully", "target_drive", realDev)
	} else {
		slog.DebugContext(ctx, "blkdiscard not supported or failed; clearing disk boundaries manually", "error", err)
	}

	// Step 3: Zero out the first 32 MiB and the last 32 MiB to destroy primary and secondary GPT/MBR/LVM headers
	file, err := os.OpenFile(realDev, os.O_WRONLY|os.O_SYNC, 0660)
	if err != nil {
		return fmt.Errorf("failed opening %s for boundary zeroing: %w", realDev, err)
	}
	defer file.Close()

	zeroChunk := make([]byte, 1024*1024) // 1 MiB chunk of zeros

	// Write first 32 MiB
	for i := 0; i < 32; i++ {
		if _, err := file.Write(zeroChunk); err != nil {
			break
		}
	}

	// Find disk size to zero last 32 MiB
	if diskSize, err := file.Seek(0, io.SeekEnd); err == nil && diskSize > 64*1024*1024 {
		startLast32MB := diskSize - 32*1024*1024
		if _, err := file.Seek(startLast32MB, io.SeekStart); err == nil {
			for i := 0; i < 32; i++ {
				if _, err := file.Write(zeroChunk); err != nil {
					break
				}
			}
		}
	}

	_ = file.Sync()

	// Step 4: Inform kernel of cleared partition table
	_ = exec.CommandContext(ctx, "partprobe", realDev).Run()
	_ = exec.CommandContext(ctx, "blockdev", "--rereadpt", realDev).Run()
	_ = exec.CommandContext(ctx, "udevadm", "settle").Run()
	_ = exec.CommandContext(ctx, "mdev", "-s").Run()

	slog.InfoContext(ctx, "target block storage device pre-wiped and prepared", "target_drive", targetDrive, "real_dev", realDev)
	return nil
}
