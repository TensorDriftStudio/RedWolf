package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

var imageHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   20 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 5 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	},
}

// DownloadStatus tracks live download progress of an OS image.
type DownloadStatus struct {
	OS          domain.OperatingSystem `json:"os"`
	Filename    string                 `json:"filename"`
	TotalBytes  int64                  `json:"totalBytes"`
	CopiedBytes int64                  `json:"copiedBytes"`
	Progress    int                    `json:"progress"`
	Status      string                 `json:"status"` // "downloading", "converting", "completed", "error"
	Error       string                 `json:"error,omitempty"`
}

// OSImageInfo describes an OS distribution raw image and its availability on disk.
type OSImageInfo struct {
	Filename       string                 `json:"filename"`
	OS             domain.OperatingSystem `json:"os"`
	DisplayName    string                 `json:"displayName"`
	SizeBytes      int64                  `json:"sizeBytes"`
	Present        bool                   `json:"present"`
	DownloadURL    string                 `json:"downloadUrl"`
	DownloadStatus *DownloadStatus        `json:"downloadStatus,omitempty"`
}

type imageTarget struct {
	os          domain.OperatingSystem
	displayName string
	candidates  []string
	downloadURL string
}

var supportedTargets = []imageTarget{
	{
		os:          domain.OSAlmaLinux9,
		displayName: "AlmaLinux 9 (Enterprise LTS)",
		candidates:  []string{"almalinux-9-genericcloud.raw.zstd", "almalinux-9-genericcloud.raw.zst", "almalinux-9-genericcloud.raw", "almalinux-9-genericcloud.qcow2"},
		downloadURL: "https://repo.almalinux.org/almalinux/9/cloud/x86_64/images/AlmaLinux-9-GenericCloud-latest.x86_64.qcow2",
	},
	{
		os:          domain.OSDebian12,
		displayName: "Debian 12 Bookworm (Stable LTS)",
		candidates:  []string{"debian-12-genericcloud.raw.zstd", "debian-12-genericcloud-amd64.raw", "debian-12-genericcloud.raw", "debian-12-genericcloud.raw.zst"},
		downloadURL: "https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-genericcloud-amd64.raw",
	},
	{
		os:          domain.OSAlmaLinux8,
		displayName: "AlmaLinux 8 (Legacy Enterprise)",
		candidates:  []string{"almalinux-8-genericcloud.raw.zstd", "almalinux-8-genericcloud.raw.zst", "almalinux-8-genericcloud.raw", "almalinux-8-genericcloud.qcow2"},
		downloadURL: "https://repo.almalinux.org/almalinux/8/cloud/x86_64/images/AlmaLinux-8-GenericCloud-latest.x86_64.qcow2",
	},
	{
		os:          domain.OSAlmaLinux10,
		displayName: "AlmaLinux 10 (Enterprise LTS)",
		candidates:  []string{"almalinux-10-genericcloud.raw.zstd", "almalinux-10-genericcloud.raw.zst", "almalinux-10-genericcloud.raw", "almalinux-10-genericcloud.qcow2"},
		downloadURL: "https://repo.almalinux.org/almalinux/10/cloud/x86_64/images/AlmaLinux-10-GenericCloud-latest.x86_64.qcow2",
	},
	{
		os:          domain.OSDebian13,
		displayName: "Debian 13 Trixie (Stable LTS)",
		candidates:  []string{"debian-13-genericcloud.raw.zstd", "debian-13-genericcloud-amd64.raw", "debian-13-genericcloud.raw", "debian-13-genericcloud.raw.zst"},
		downloadURL: "https://cloud.debian.org/images/cloud/trixie/latest/debian-13-genericcloud-amd64.raw",
	},
	{
		os:          domain.OSUbuntu2404,
		displayName: "Ubuntu 24.04 LTS Noble (Enterprise LTS)",
		candidates:  []string{"ubuntu-24.04-server-cloudimg-amd64.raw.zstd", "ubuntu-24.04-server-cloudimg-amd64.raw.zst", "ubuntu-24.04-server-cloudimg-amd64.raw", "noble-server-cloudimg-amd64.raw", "noble-server-cloudimg-amd64.img", "ubuntu-24.04-server-cloudimg-amd64.img"},
		downloadURL: "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.raw",
	},
	{
		os:          domain.OSUbuntu2204,
		displayName: "Ubuntu 22.04 LTS Jammy (Enterprise LTS)",
		candidates:  []string{"ubuntu-22.04-server-cloudimg-amd64.raw.zstd", "ubuntu-22.04-server-cloudimg-amd64.raw.zst", "ubuntu-22.04-server-cloudimg-amd64.raw", "jammy-server-cloudimg-amd64.raw", "jammy-server-cloudimg-amd64.img", "ubuntu-22.04-server-cloudimg-amd64.img"},
		downloadURL: "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.raw",
	},
}

// getOSSlug converts domain.OperatingSystem to a filesystem and URL friendly slug (e.g. "almalinux-9", "debian-12", "ubuntu-24.04").
func getOSSlug(osType domain.OperatingSystem) string {
	switch osType {
	case domain.OSAlmaLinux8:
		return "almalinux-8"
	case domain.OSAlmaLinux9:
		return "almalinux-9"
	case domain.OSAlmaLinux10:
		return "almalinux-10"
	case domain.OSDebian12:
		return "debian-12"
	case domain.OSDebian13:
		return "debian-13"
	case domain.OSUbuntu2404:
		return "ubuntu-24.04"
	case domain.OSUbuntu2204:
		return "ubuntu-22.04"
	default:
		s := strings.ToLower(string(osType))
		return strings.ReplaceAll(s, " ", "-")
	}
}

// ImageCatalogService discovers and manages OS raw images available for streaming.
type ImageCatalogService struct {
	imageDir  string
	mu        sync.RWMutex
	downloads map[domain.OperatingSystem]*DownloadStatus
}

// NewImageCatalogService initializes the OS image catalog.
func NewImageCatalogService(imageDir string) *ImageCatalogService {
	return &ImageCatalogService{
		imageDir:  imageDir,
		downloads: make(map[domain.OperatingSystem]*DownloadStatus),
	}
}

// SetImageDir updates the primary image storage directory.
func (s *ImageCatalogService) SetImageDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.imageDir = dir
}

// resolveWritableImageDirLocked checks candidate directories and returns the first writable directory.
// Caller must hold s.mu (Lock or RLock).
func (s *ImageCatalogService) resolveWritableImageDirLocked() string {
	primary := s.imageDir
	candidates := []string{primary, "/var/lib/redwolf/images", "data/images"}
	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0755); err == nil {
			testFile := filepath.Join(dir, fmt.Sprintf(".write_test_%d", time.Now().UnixNano()))
			if err := os.WriteFile(testFile, []byte("ok"), 0644); err == nil {
				_ = os.Remove(testFile)
				return dir
			}
		}
	}
	return "data/images"
}

// ListImages inspects the filesystem and returns the status of all supported OS images.
func (s *ImageCatalogService) ListImages(ctx context.Context) ([]OSImageInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var searchDirs []string
	if s.imageDir != "" {
		searchDirs = append(searchDirs, s.imageDir)
	}
	// Only include standard appliance fallbacks if imageDir is empty or matches standard paths
	if s.imageDir == "" || s.imageDir == "/var/lib/redwolf/images" || s.imageDir == "data/images" {
		searchDirs = append(searchDirs, "/var/lib/redwolf/images", "data/images", "/usr/share/redwolf/images")
	}

	var results []OSImageInfo
	for _, t := range supportedTargets {
		var foundFilename string
		var foundSize int64
		present := false

		for _, candidate := range t.candidates {
			for _, dir := range searchDirs {
				if dir == "" {
					continue
				}
				p := filepath.Join(dir, candidate)
				if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
					foundFilename = candidate
					foundSize = fi.Size()
					present = true
					break
				}
			}
			if present {
				break
			}
		}

		if !present {
			foundFilename = t.candidates[0]
		}

		dlStatus := s.downloads[t.os]

		results = append(results, OSImageInfo{
			Filename:       foundFilename,
			OS:             t.os,
			DisplayName:    t.displayName,
			SizeBytes:      foundSize,
			Present:        present,
			DownloadURL:    t.downloadURL,
			DownloadStatus: dlStatus,
		})
	}

	return results, nil
}

// DownloadImage initiates background download of the specified distribution cloud raw image.
func (s *ImageCatalogService) DownloadImage(ctx context.Context, osType domain.OperatingSystem) (*DownloadStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var target *imageTarget
	for _, t := range supportedTargets {
		if t.os == osType {
			target = &t
			break
		}
	}

	if target == nil {
		return nil, fmt.Errorf("unsupported operating system: %s", osType)
	}

	if target.downloadURL == "" {
		return nil, fmt.Errorf("no official cloud image repository configured for %s", osType)
	}

	// Check if already downloading or converting
	if current, exists := s.downloads[osType]; exists && (current.Status == "downloading" || current.Status == "converting") {
		return current, nil
	}

	slug := getOSSlug(osType)
	targetDir := s.resolveWritableImageDirLocked()
	urlLower := strings.ToLower(target.downloadURL)
	isQcow2 := strings.HasSuffix(urlLower, ".qcow2")

	var destFilename string
	var tempFilename string
	if isQcow2 {
		destFilename = fmt.Sprintf("%s-genericcloud.raw.zstd", slug)
		tempFilename = fmt.Sprintf("%s-genericcloud.upstream.qcow2.part", slug)
	} else {
		destFilename = fmt.Sprintf("%s-genericcloud.raw.zstd", slug)
		tempFilename = fmt.Sprintf("%s-genericcloud.upstream.raw.part", slug)
	}

	destPath := filepath.Join(targetDir, destFilename)
	tempPath := filepath.Join(targetDir, tempFilename)

	status := &DownloadStatus{
		OS:         osType,
		Filename:   destFilename,
		Status:     "downloading",
		Progress:   0,
		TotalBytes: 0,
	}
	s.downloads[osType] = status

	// Run download and conversion asynchronously in detached background goroutine
	go func(targetURL, downloadTempPath, finalDestPath, destDir, imgSlug string, dlStatus *DownloadStatus, targetOS domain.OperatingSystem, qcowSource bool) {
		slog.Info("starting official cloud image background download", "os", targetOS, "url", targetURL, "destination", finalDestPath)

		if err := os.MkdirAll(destDir, 0755); err != nil {
			s.mu.Lock()
			dlStatus.Status = "error"
			dlStatus.Error = fmt.Sprintf("failed creating image directory: %v", err)
			s.mu.Unlock()
			return
		}

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, targetURL, nil)
		if err != nil {
			s.mu.Lock()
			dlStatus.Status = "error"
			dlStatus.Error = err.Error()
			s.mu.Unlock()
			return
		}
		req.Header.Set("User-Agent", "RedWolf-Provisioner/1.2.0 (+https://github.com/TensorDriftStudio/RedWolf)")

		resp, err := imageHTTPClient.Do(req)
		if err != nil {
			s.mu.Lock()
			dlStatus.Status = "error"
			dlStatus.Error = err.Error()
			s.mu.Unlock()
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			s.mu.Lock()
			dlStatus.Status = "error"
			dlStatus.Error = fmt.Sprintf("server responded with status %d", resp.StatusCode)
			s.mu.Unlock()
			return
		}

		s.mu.Lock()
		dlStatus.TotalBytes = resp.ContentLength
		s.mu.Unlock()

		outFile, err := os.OpenFile(downloadTempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			s.mu.Lock()
			dlStatus.Status = "error"
			dlStatus.Error = err.Error()
			s.mu.Unlock()
			return
		}

		buf := make([]byte, 512*1024) // 512KB buffer for high-throughput streaming
		var copied int64
		lastLog := time.Now()

		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				_, writeErr := outFile.Write(buf[:n])
				if writeErr != nil {
					_ = outFile.Close()
					_ = os.Remove(downloadTempPath)
					s.mu.Lock()
					dlStatus.Status = "error"
					dlStatus.Error = writeErr.Error()
					s.mu.Unlock()
					return
				}
				copied += int64(n)

				if time.Since(lastLog) > 300*time.Millisecond {
					s.mu.Lock()
					dlStatus.CopiedBytes = copied
					if dlStatus.TotalBytes > 0 {
						dlStatus.Progress = int((copied * 80) / dlStatus.TotalBytes)
					}
					s.mu.Unlock()
					lastLog = time.Now()
				}
			}

			if readErr != nil {
				if readErr == io.EOF {
					break
				}
				_ = outFile.Close()
				_ = os.Remove(downloadTempPath)
				s.mu.Lock()
				dlStatus.Status = "error"
				dlStatus.Error = readErr.Error()
				s.mu.Unlock()
				return
			}
		}

		_ = outFile.Close()

		var finalFilename string
		var finalSize int64

		if qcowSource {
			// Convert QCOW2 to raw sparse disk format
			if qemuPath, err := exec.LookPath("qemu-img"); err == nil {
				slog.Info("converting downloaded QCOW2 image to raw sparse disk format", "os", targetOS, "temp", downloadTempPath)
				s.mu.Lock()
				dlStatus.Status = "converting"
				dlStatus.Progress = 85
				s.mu.Unlock()

				rawFilename := fmt.Sprintf("%s-genericcloud.raw", imgSlug)
				rawPath := filepath.Join(destDir, rawFilename)
				cmd := exec.CommandContext(context.Background(), qemuPath, "convert", "-f", "qcow2", "-O", "raw", downloadTempPath, rawPath)
				if out, err := cmd.CombinedOutput(); err != nil {
					s.mu.Lock()
					dlStatus.Status = "error"
					dlStatus.Error = fmt.Sprintf("qemu-img conversion failed: %s (%v)", string(out), err)
					s.mu.Unlock()
					_ = os.Remove(downloadTempPath)
					return
				}
				_ = os.Remove(downloadTempPath)

				// Compress raw image with zstd if available
				if zstdPath, err := exec.LookPath("zstd"); err == nil {
					slog.Info("compressing raw disk image with zstd", "os", targetOS, "raw", rawPath)
					s.mu.Lock()
					dlStatus.Progress = 92
					s.mu.Unlock()

					zstdFilename := fmt.Sprintf("%s-genericcloud.raw.zstd", imgSlug)
					zstdPathDest := filepath.Join(destDir, zstdFilename)
					zstdCmd := exec.CommandContext(context.Background(), zstdPath, "--rm", "-3", rawPath, "-o", zstdPathDest)
					if out, err := zstdCmd.CombinedOutput(); err != nil {
						slog.Warn("zstd compression failed, keeping uncompressed raw image", "error", err, "output", string(out))
						finalFilename = rawFilename
					} else {
						finalFilename = zstdFilename
					}
				} else {
					finalFilename = rawFilename
				}
			} else {
				// qemu-img not available, keep downloaded image as qcow2 with normalized lowercase filename
				qcowFilename := fmt.Sprintf("%s-genericcloud.qcow2", imgSlug)
				qcowPath := filepath.Join(destDir, qcowFilename)
				if err := os.Rename(downloadTempPath, qcowPath); err != nil {
					s.mu.Lock()
					dlStatus.Status = "error"
					dlStatus.Error = err.Error()
					s.mu.Unlock()
					return
				}
				finalFilename = qcowFilename
			}
		} else {
			// Direct raw image (Debian 12 / Debian 13)
			rawFilename := fmt.Sprintf("%s-genericcloud.raw", imgSlug)
			rawPath := filepath.Join(destDir, rawFilename)
			if err := os.Rename(downloadTempPath, rawPath); err != nil {
				s.mu.Lock()
				dlStatus.Status = "error"
				dlStatus.Error = err.Error()
				s.mu.Unlock()
				return
			}

			// If zstd compressor is available, compress raw sparse disk into high-speed streaming format
			if zstdPath, err := exec.LookPath("zstd"); err == nil {
				slog.Info("compressing downloaded Debian raw disk image with zstd", "os", targetOS, "raw", rawPath)
				s.mu.Lock()
				dlStatus.Status = "converting"
				dlStatus.Progress = 90
				s.mu.Unlock()

				zstdFilename := fmt.Sprintf("%s-genericcloud.raw.zstd", imgSlug)
				zstdPathDest := filepath.Join(destDir, zstdFilename)
				zstdCmd := exec.CommandContext(context.Background(), zstdPath, "--rm", "-3", rawPath, "-o", zstdPathDest)
				if out, err := zstdCmd.CombinedOutput(); err != nil {
					slog.Warn("zstd compression failed, keeping uncompressed raw image", "error", err, "output", string(out))
					finalFilename = rawFilename
				} else {
					finalFilename = zstdFilename
				}
			} else {
				finalFilename = rawFilename
			}
		}

		finalFilePath := filepath.Join(destDir, finalFilename)
		if fi, err := os.Stat(finalFilePath); err == nil {
			finalSize = fi.Size()
		} else {
			finalSize = copied
		}

		s.mu.Lock()
		dlStatus.Status = "completed"
		dlStatus.Progress = 100
		dlStatus.Filename = finalFilename
		dlStatus.CopiedBytes = finalSize
		dlStatus.TotalBytes = finalSize
		s.mu.Unlock()

		slog.Info("completed cloud image download and caching", "os", targetOS, "filename", finalFilename, "size_bytes", finalSize, "path", finalFilePath)
	}(target.downloadURL, tempPath, destPath, targetDir, slug, status, osType, isQcow2)

	return status, nil
}

// GetDownloadStatuses returns all active or recent download statuses.
func (s *ImageCatalogService) GetDownloadStatuses() map[domain.OperatingSystem]*DownloadStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	copied := make(map[domain.OperatingSystem]*DownloadStatus, len(s.downloads))
	for k, v := range s.downloads {
		statusCopy := *v
		copied[k] = &statusCopy
	}
	return copied
}

// GetImageFilename returns the filename of the cached OS image for the specified distribution, or the default candidate name.
func (s *ImageCatalogService) GetImageFilename(ctx context.Context, osType domain.OperatingSystem) (string, error) {
	images, err := s.ListImages(ctx)
	if err != nil {
		return "", err
	}
	for _, img := range images {
		if img.OS == osType && img.Present {
			return img.Filename, nil
		}
	}
	for _, t := range supportedTargets {
		if t.os == osType {
			return t.candidates[0], nil
		}
	}
	return "", fmt.Errorf("unsupported operating system: %s", osType)
}

// IsImagePresent checks if an image for the given OS is currently cached in storage.
func (s *ImageCatalogService) IsImagePresent(ctx context.Context, osType domain.OperatingSystem) bool {
	images, err := s.ListImages(ctx)
	if err != nil {
		return false
	}
	for _, img := range images {
		if img.OS == osType && img.Present {
			return true
		}
	}
	return false
}
