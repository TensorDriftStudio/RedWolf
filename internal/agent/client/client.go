package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// AgentClient interacts with RedWolf Core server endpoints.
type AgentClient struct {
	serverURL  string
	httpClient *http.Client
}

// NewAgentClient creates an initialized HTTP client for RedWolf Core.
func NewAgentClient(serverURL string) *AgentClient {
	serverURL = strings.TrimRight(serverURL, "/")
	return &AgentClient{
		serverURL: serverURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SubmitTelemetry posts the discovered server hardware inventory to the core engine.
func (c *AgentClient) SubmitTelemetry(ctx context.Context, node *domain.ServerNode) (*domain.ServerNode, error) {
	url := fmt.Sprintf("%s/api/nodes/telemetry", c.serverURL)

	payload, err := json.Marshal(node)
	if err != nil {
		return nil, fmt.Errorf("failed marshaling telemetry payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telemetry post failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server responded with status %d", resp.StatusCode)
	}

	var registered domain.ServerNode
	if err := json.NewDecoder(resp.Body).Decode(&registered); err != nil {
		return nil, fmt.Errorf("failed decoding registered node response: %w", err)
	}

	slog.InfoContext(ctx, "telemetry successfully registered with RedWolf Core",
		"node_id", registered.ID,
		"server_url", c.serverURL,
	)

	return &registered, nil
}

// PollTask checks if a pending deployment task has been authorized for this node's MAC address.
func (c *AgentClient) PollTask(ctx context.Context, mac string) (*domain.DeploymentTask, error) {
	url := fmt.Sprintf("%s/api/nodes/task/by-mac/%s", c.serverURL, mac)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // No task currently pending
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var task domain.DeploymentTask
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		return nil, fmt.Errorf("failed decoding deployment task: %w", err)
	}

	// Resolve relative ImageURL to absolute URL if needed
	if strings.HasPrefix(task.ImageURL, "/") {
		task.ImageURL = c.serverURL + task.ImageURL
	}

	return &task, nil
}

// Report sends live progress updates to RedWolf Core.
func (c *AgentClient) Report(ctx context.Context, nodeID string, progress int, stage, logMsg string) error {
	url := fmt.Sprintf("%s/api/nodes/%s/progress", c.serverURL, nodeID)

	body := map[string]any{
		"progress": progress,
		"stage":    stage,
		"log":      logMsg,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
