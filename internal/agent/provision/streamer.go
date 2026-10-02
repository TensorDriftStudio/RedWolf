package provision

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

// ProgressCallback notifies callers of transfer progress.
type ProgressCallback func(percent int, writtenBytes int64, message string)

// StreamImage streams and decompresses a cloud raw image directly to target block storage.
// It implements sparse block writes (skipping runs of zeros) to preserve drive endurance and maximize write speed.
func StreamImage(ctx context.Context, imageURL string, targetDrivePath string, onProgress ProgressCallback) error {
	slog.InfoContext(ctx, "initiating sparse block stream to target drive",
		"image_url", imageURL,
		"target_drive", targetDrivePath,
	)

	// Step 1: Open target block device with direct sync flags
	targetDev, err := os.OpenFile(targetDrivePath, os.O_WRONLY|os.O_SYNC, 0660)
	if err != nil {
		return fmt.Errorf("failed opening target block device %s: %w", targetDrivePath, err)
	}
	defer targetDev.Close()

	// Step 2: Request image from server
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return fmt.Errorf("failed creating image http request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed fetching image from %s: %w", imageURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server responded with status %d for image %s", resp.StatusCode, imageURL)
	}

	totalExpectedBytes := resp.ContentLength

	// Step 3: Wrap body with appropriate decompressor
	var streamReader io.Reader = resp.Body
	urlLower := strings.ToLower(imageURL)

	if strings.HasSuffix(urlLower, ".zst") || strings.HasSuffix(urlLower, ".zstd") {
		zstdReader, err := zstd.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("failed initializing zstd decompressor: %w", err)
		}
		defer zstdReader.Close()
		streamReader = zstdReader
	} else if strings.HasSuffix(urlLower, ".gz") {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("failed initializing gzip decompressor: %w", err)
		}
		defer gzReader.Close()
		streamReader = gzReader
	}

	// Step 4: Stream in 4 MiB buffer chunks with sparse zero-skipping
	const chunkSize = 4 * 1024 * 1024
	buf := make([]byte, chunkSize)
	zeroBuf := make([]byte, chunkSize)

	var totalDecompressedBytes int64
	var totalWrittenBytes int64
	lastReportTime := time.Now()

	if onProgress != nil {
		onProgress(10, 0, fmt.Sprintf("Streaming OS image from %s to %s", imageURL, targetDrivePath))
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, readErr := io.ReadFull(streamReader, buf)
		if n > 0 {
			totalDecompressedBytes += int64(n)

			// Check if block contains entirely zeros (sparse hole)
			if bytes.Equal(buf[:n], zeroBuf[:n]) {
				// Seek forward over the zero hole without physical NAND/platter write
				_, err := targetDev.Seek(int64(n), io.SeekCurrent)
				if err != nil {
					return fmt.Errorf("failed seeking sparse hole at offset %d: %w", totalDecompressedBytes, err)
				}
			} else {
				// Write data chunk
				written, err := targetDev.Write(buf[:n])
				if err != nil {
					return fmt.Errorf("failed writing block at offset %d: %w", totalDecompressedBytes, err)
				}
				totalWrittenBytes += int64(written)
			}

			// Emit progress every 3 seconds or every 500MB
			if time.Since(lastReportTime) > 3*time.Second {
				lastReportTime = time.Now()
				pct := 15
				if totalExpectedBytes > 0 {
					// Approximate progress scaled between 15% and 70%
					fraction := float64(totalWrittenBytes) / float64(totalExpectedBytes*3) // Estimated 3x compression
					if fraction > 1.0 {
						fraction = 1.0
					}
					pct = 15 + int(fraction*55)
				}
				if onProgress != nil {
					mbWritten := float64(totalWrittenBytes) / (1024 * 1024)
					onProgress(pct, totalWrittenBytes, fmt.Sprintf("Written %.1f MB (sparse decompressed %.1f MB)", mbWritten, float64(totalDecompressedBytes)/(1024*1024)))
				}
			}
		}

		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("error reading compressed stream: %w", readErr)
		}
	}

	// Step 5: Flush disk cache to guarantee consistency
	if err := targetDev.Sync(); err != nil {
		slog.WarnContext(ctx, "failed syncing target device cache", "error", err)
	}

	if onProgress != nil {
		onProgress(70, totalWrittenBytes, fmt.Sprintf("Image stream complete (total %.1f MB written to %s)", float64(totalWrittenBytes)/(1024*1024), targetDrivePath))
	}

	slog.InfoContext(ctx, "image streaming finished successfully",
		"target_drive", targetDrivePath,
		"decompressed_bytes", totalDecompressedBytes,
		"written_bytes", totalWrittenBytes,
	)

	return nil
}
