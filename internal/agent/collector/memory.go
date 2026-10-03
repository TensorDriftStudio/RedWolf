package collector

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// CollectMemory inspects physical RAM capacity and DIMM configuration.
func CollectMemory(ctx context.Context) (*domain.MemoryInfo, error) {
	info := &domain.MemoryInfo{
		TotalBytes: 0,
		TotalHuman: "0 GB",
		SlotsUsed:  1,
		SlotsTotal: 1,
		Type:       "DDR4",
		SpeedMHz:   3200,
	}

	// 1. Read MemTotal from /proc/meminfo
	meminfoFile, err := os.Open("/proc/meminfo")
	if err == nil {
		scanner := bufio.NewScanner(meminfoFile)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if kb, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
						info.TotalBytes = kb * 1024
					}
				}
				break
			}
		}
		if err := scanner.Err(); err != nil {
			slog.WarnContext(ctx, "scanner error reading /proc/meminfo", "error", err)
		}
		meminfoFile.Close()
	}

	// Format human readable size in GB
	gb := float64(info.TotalBytes) / (1024 * 1024 * 1024)
	info.TotalHuman = fmt.Sprintf("%.0f GB", gb)

	// 2. Query dmidecode type 17 for physical DIMM slot topology
	cmd := exec.CommandContext(ctx, "dmidecode", "-t", "17")
	out, err := cmd.Output()
	if err == nil {
		parseDMIMemory(string(out), info)
	}

	slog.InfoContext(ctx, "memory telemetry collected",
		"total_human", info.TotalHuman,
		"slots_used", info.SlotsUsed,
		"slots_total", info.SlotsTotal,
		"type", info.Type,
		"speed_mhz", info.SpeedMHz,
	)

	return info, nil
}

func parseDMIMemory(dmiOutput string, info *domain.MemoryInfo) {
	scanner := bufio.NewScanner(strings.NewReader(dmiOutput))
	totalSlots := 0
	usedSlots := 0
	dimmType := ""
	speedMHz := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "Size":
			totalSlots++
			if !strings.EqualFold(val, "No Module Installed") && !strings.EqualFold(val, "Not Installed") {
				usedSlots++
			}
		case "Type":
			if val != "Unknown" && val != "Other" && dimmType == "" {
				dimmType = val
			}
		case "Configured Memory Speed", "Speed":
			if strings.HasSuffix(val, "MT/s") || strings.HasSuffix(val, "MHz") {
				fields := strings.Fields(val)
				if len(fields) > 0 {
					if s, err := strconv.Atoi(fields[0]); err == nil && s > speedMHz {
						speedMHz = s
					}
				}
			}
		}
	}

	if totalSlots > 0 {
		info.SlotsTotal = totalSlots
	}
	if usedSlots > 0 {
		info.SlotsUsed = usedSlots
	}
	if dimmType != "" {
		info.Type = dimmType
	}
	if speedMHz > 0 {
		info.SpeedMHz = speedMHz
	}

	if info.SlotsUsed > 0 {
		info.TotalHuman = fmt.Sprintf("%.0f GB (%dx DIMM %s-%d)",
			float64(info.TotalBytes)/(1024*1024*1024),
			info.SlotsUsed,
			info.Type,
			info.SpeedMHz,
		)
	}
}
