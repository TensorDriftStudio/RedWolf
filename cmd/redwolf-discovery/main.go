package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/tensordriftstudio/redwolf/internal/agent/client"
	"github.com/tensordriftstudio/redwolf/internal/agent/collector"
	"github.com/tensordriftstudio/redwolf/internal/agent/provision"
)

func main() {
	serverFlag := flag.String("server", "", "RedWolf Core server URL (e.g. http://192.168.0.250:8080)")
	flag.Parse()

	// Initialize structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	slog.InfoContext(ctx, "RedWolf in-memory discovery agent starting")

	// Determine server URL: CLI flag -> ENV -> /proc/cmdline
	serverURL := resolveServerURL(*serverFlag)
	if serverURL == "" {
		slog.ErrorContext(ctx, "could not determine RedWolf server URL; specify -server or set redwolf_server= in kernel cmdline")
		os.Exit(1)
	}

	slog.InfoContext(ctx, "configured RedWolf Core target", "server_url", serverURL)
	agentClient := client.NewAgentClient(serverURL)

	// Step 1: Collect hardware telemetry
	node, err := collector.CollectAll(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed collecting hardware telemetry", "error", err)
		os.Exit(1)
	}

	bootMAC := node.BootMAC()
	slog.InfoContext(ctx, "hardware inventory cataloged successfully",
		"vendor", node.Vendor,
		"model", node.Model,
		"serial", node.SerialNumber,
		"boot_mac", bootMAC,
	)

	// Step 2: Register telemetry with Core engine (retry loop for network readiness)
	var registeredNodeID string
	for {
		registered, err := agentClient.SubmitTelemetry(ctx, node)
		if err == nil {
			registeredNodeID = registered.ID
			slog.InfoContext(ctx, "node registered with core engine", "node_id", registeredNodeID)
			break
		}

		slog.WarnContext(ctx, "failed submitting telemetry; retrying in 5s...", "error", err)
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, "agent terminated by signal")
			return
		case <-time.After(5 * time.Second):
		}
	}

	// Step 3: Enter deployment polling loop
	slog.InfoContext(ctx, "entering deployment task polling loop; waiting for operator authorization", "mac", bootMAC)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, "agent shutdown requested")
			return
		case <-ticker.C:
			task, err := agentClient.PollTask(ctx, bootMAC)
			if err != nil {
				slog.DebugContext(ctx, "poll task check error", "error", err)
				continue
			}

			if task == nil {
				// No task yet
				continue
			}

			slog.InfoContext(ctx, "received authorized deployment task",
				"task_id", task.TaskID,
				"os", task.OS,
				"target_drive", task.TargetDrivePath,
			)

			// Step 4: Execute deployment workflow
			if err := provision.ExecuteDeployment(ctx, task, bootMAC, agentClient); err != nil {
				slog.ErrorContext(ctx, "fatal error during bare-metal deployment execution", "error", err)
			}
			return
		}
	}
}

func resolveServerURL(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if envVal := os.Getenv("REDWOLF_SERVER"); envVal != "" {
		return envVal
	}

	// Check /proc/cmdline for redwolf_server=... or redwolf.server=...
	data, err := os.ReadFile("/proc/cmdline")
	if err == nil {
		for _, param := range strings.Fields(string(data)) {
			if strings.HasPrefix(param, "redwolf_server=") {
				return strings.TrimPrefix(param, "redwolf_server=")
			}
			if strings.HasPrefix(param, "redwolf.server=") {
				return strings.TrimPrefix(param, "redwolf.server=")
			}
		}
	}

	// Fallback: detect default gateway from system routing table
	if out, err := exec.Command("ip", "route", "show", "default").Output(); err == nil {
		fields := strings.Fields(string(out))
		for i, f := range fields {
			if f == "via" && i+1 < len(fields) {
				gw := fields[i+1]
				return fmt.Sprintf("http://%s:8080", gw)
			}
		}
	}

	return ""
}
