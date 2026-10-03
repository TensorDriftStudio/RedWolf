package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// DownloadStatus tracks live download progress of an OS image.
type DownloadStatus struct {
	OS          domain.OperatingSystem `json:"os"`
	Filename    string                 `json:"filename"`
	TotalBytes  int64                  `json:"totalBytes"`
	CopiedBytes int64                  `json:"copiedBytes"`
	Progress    int                    `json:"progress"`
	Status      string                 `json:"status"` // "downloading", "completed", "error"
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
		candidates:  []string{"almalinux-9-genericcloud.raw.zstd", "AlmaLinux-9-GenericCloud-latest.x86_64.raw.zst", "almalinux-9-genericcloud.raw", "AlmaLinux-9-GenericCloud-latest.x86_64.qcow2"},
		downloadURL: "https://repo.almalinux.org/almalinux/9/cloud/x86_64/images/AlmaLinux-9-GenericCloud-latest.x86_64.qcow2",
	},
	{
		os:          domain.OSDebian12,
		displayName: "Debian 12 Bookworm (Stable LTS)",
		candidates:  []string{"debian-12-genericcloud.raw.zstd", "debian-12-genericcloud-amd64.raw.zst", "debian-12-genericcloud-amd64.raw", "debian-12-genericcloud.raw"},
		downloadURL: "https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-genericcloud-amd64.raw",
	},
	{
		os:          domain.OSAlmaLinux8,
		displayName: "AlmaLinux 8 (Legacy Enterprise)",
		candidates:  []string{"almalinux-8-genericcloud.raw.zstd", "AlmaLinux-8-GenericCloud-latest.x86_64.raw.zst", "almalinux-8-genericcloud.raw", "AlmaLinux-8-GenericCloud-latest.x86_64.qcow2"},
		downloadURL: "https://repo.almalinux.org/almalinux/8/cloud/x86_64/images/AlmaLinux-8-GenericCloud-latest.x86_64.qcow2",
	},
	{
		os:          domain.OSAlmaLinux10,
		displayName: "AlmaLinux 10 (Technology Preview)",
		candidates:  []string{"almalinux-10-genericcloud.raw.zstd", "almalinux-10-genericcloud.raw"},
		downloadURL: "",
	},
	{
		os:          domain.OSDebian13,
		displayName: "Debian 13 Trixie (Testing)",
		candidates:  []string{"debian-13-genericcloud.raw.zstd", "debian-13-genericcloud.raw"},
		downloadURL: "",
	},
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

// ListImages inspects the filesystem and returns the status of all supported OS images.
func (s *ImageCatalogService) ListImages(ctx context.Context) ([]OSImageInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	searchDirs := []string{
		s.imageDir,
		"data/images",
		"/var/lib/redwolf/images",
		"/usr/share/redwolf/images",
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

	// Check if already downloading
	if current, exists := s.downloads[osType]; exists && current.Status == "downloading" {
		return current, nil
	}

	destFilename := target.candidates[0]
	urlLower := strings.ToLower(target.downloadURL)
	if strings.HasSuffix(urlLower, ".raw") {
		for _, cand := range target.candidates {
			if strings.HasSuffix(cand, ".raw") && !strings.HasSuffix(cand, ".raw.zstd") && !strings.HasSuffix(cand, ".raw.zst") {
				destFilename = cand
				break
			}
		}
	} else if strings.HasSuffix(urlLower, ".qcow2") {
		for _, cand := range target.candidates {
			if strings.HasSuffix(cand, ".qcow2") {
				destFilename = cand
				break
			}
		}
	}

	destPath := filepath.Join(s.imageDir, destFilename)
	partPath := destPath + ".part"

	status := &DownloadStatus{
		OS:         osType,
		Filename:   destFilename,
		Status:     "downloading",
		Progress:   0,
		TotalBytes: 0,
	}
	s.downloads[osType] = status

	// Run download asynchronously in detached context
	go func(targetURL, finalPath, tempPath string, dlStatus *DownloadStatus, targetOS domain.OperatingSystem) {
		slog.Info("starting official cloud image background download", "os", targetOS, "url", targetURL, "destination", finalPath)

		if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
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

		resp, err := http.DefaultClient.Do(req)
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

		outFile, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
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
					_ = os.Remove(tempPath)
					s.mu.Lock()
					dlStatus.Status = "error"
					dlStatus.Error = writeErr.Error()
					s.mu.Unlock()
					return
				}
				copied += int64(n)

				if time.Since(lastLog) > 500*time.Millisecond {
					s.mu.Lock()
					dlStatus.CopiedBytes = copied
					if dlStatus.TotalBytes > 0 {
						dlStatus.Progress = int((copied * 100) / dlStatus.TotalBytes)
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
				_ = os.Remove(tempPath)
				s.mu.Lock()
				dlStatus.Status = "error"
				dlStatus.Error = readErr.Error()
				s.mu.Unlock()
				return
			}
		}

		_ = outFile.Close()

		// Rename temp part file to final image
		if err := os.Rename(tempPath, finalPath); err != nil {
			s.mu.Lock()
			dlStatus.Status = "error"
			dlStatus.Error = err.Error()
			s.mu.Unlock()
			return
		}

		s.mu.Lock()
		dlStatus.Status = "completed"
		dlStatus.Progress = 100
		dlStatus.CopiedBytes = copied
		s.mu.Unlock()

		slog.Info("completed cloud image download and caching", "os", targetOS, "size_bytes", copied, "path", finalPath)
	}(target.downloadURL, destPath, partPath, status, osType)

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
