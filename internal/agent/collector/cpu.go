package collector

import (
	"bufio"
	"context"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// CollectCPU catalogs processor microarchitecture, sockets, and cores.
func CollectCPU(ctx context.Context) (*domain.CPUInfo, error) {
	info := &domain.CPUInfo{
		Model:            "Generic x86_64 Processor",
		Sockets:          1,
		CoresPerSocket:   1,
		ThreadsPerSocket: 1,
		TotalThreads:     runtime.NumCPU(),
		Arch:             runtime.GOARCH,
	}

	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		slog.WarnContext(ctx, "could not read /proc/cpuinfo", "error", err)
		return info, nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	physicalIDs := make(map[string]bool)
	var modelName string
	var coresPerSocket int
	var siblings int
	totalProcessors := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "model name":
			if modelName == "" {
				modelName = val
			}
		case "physical id":
			physicalIDs[val] = true
		case "cpu cores":
			if c, err := strconv.Atoi(val); err == nil && c > coresPerSocket {
				coresPerSocket = c
			}
		case "siblings":
			if s, err := strconv.Atoi(val); err == nil && s > siblings {
				siblings = s
			}
		case "processor":
			totalProcessors++
		}
	}
	if err := scanner.Err(); err != nil {
		slog.WarnContext(ctx, "scanner error reading /proc/cpuinfo", "error", err)
	}

	if modelName != "" {
		info.Model = modelName
	}
	if len(physicalIDs) > 0 {
		info.Sockets = len(physicalIDs)
	}
	if coresPerSocket > 0 {
		info.CoresPerSocket = coresPerSocket
	}
	if siblings > 0 {
		info.ThreadsPerSocket = siblings
	}
	if totalProcessors > 0 {
		info.TotalThreads = totalProcessors
	}

	slog.InfoContext(ctx, "cpu telemetry collected",
		"model", info.Model,
		"sockets", info.Sockets,
		"cores_per_socket", info.CoresPerSocket,
		"threads_total", info.TotalThreads,
	)

	return info, nil
}
