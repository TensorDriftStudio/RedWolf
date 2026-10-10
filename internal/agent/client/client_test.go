package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

func TestAgentClient_SubmitTelemetry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/nodes/telemetry" || r.Method != http.MethodPost {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var incoming domain.ServerNode
		if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		incoming.ID = "node-test-123"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(incoming)
	}))
	defer server.Close()

	c := NewAgentClient(server.URL)
	ctx := context.Background()
	node := &domain.ServerNode{
		Vendor:       domain.VendorDell,
		Model:        "PowerEdge R640",
		SerialNumber: "ABC1234",
	}

	registered, err := c.SubmitTelemetry(ctx, node)
	if err != nil {
		t.Fatalf("expected telemetry submit to succeed: %v", err)
	}
	if registered.ID != "node-test-123" {
		t.Fatalf("expected node-test-123, got %s", registered.ID)
	}
}

func TestAgentClient_PollTask_FoundAndNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/nodes/task/by-mac/52:54:00:11:22:33" {
			task := domain.DeploymentTask{
				TaskID:   "task-abc",
				NodeID:   "node-1",
				OS:       domain.OSAlmaLinux9,
				ImageURL: "/assets/images/almalinux-9.raw.zstd",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(task)
			return
		}
		http.Error(w, "no active task", http.StatusNotFound)
	}))
	defer server.Close()

	c := NewAgentClient(server.URL)
	ctx := context.Background()

	// 1. Task found
	task, err := c.PollTask(ctx, "52:54:00:11:22:33")
	if err != nil {
		t.Fatalf("expected PollTask to succeed: %v", err)
	}
	if task == nil {
		t.Fatal("expected non-nil task")
	}
	if task.TaskID != "task-abc" {
		t.Fatalf("expected task-abc, got %s", task.TaskID)
	}
	expectedURL := server.URL + "/assets/images/almalinux-9.raw.zstd"
	if task.ImageURL != expectedURL {
		t.Fatalf("expected ImageURL %s, got %s", expectedURL, task.ImageURL)
	}

	// 2. Task not found (404) returns (nil, nil)
	notFoundTask, err := c.PollTask(ctx, "00:00:00:00:00:00")
	if err != nil {
		t.Fatalf("expected nil error for 404, got %v", err)
	}
	if notFoundTask != nil {
		t.Fatalf("expected nil task for 404, got %+v", notFoundTask)
	}
}

func TestAgentClient_Report(t *testing.T) {
	var receivedProgress int
	var receivedStage string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/nodes/node-1/progress" || r.Method != http.MethodPost {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var payload struct {
			Progress int    `json:"progress"`
			Stage    string `json:"stage"`
			Log      string `json:"log"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		receivedProgress = payload.Progress
		receivedStage = payload.Stage
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewAgentClient(server.URL)
	ctx := context.Background()
	err := c.Report(ctx, "node-1", 50, "Partitioning drive", "Creating GPT tables")
	if err != nil {
		t.Fatalf("expected Report to succeed: %v", err)
	}
	if receivedProgress != 50 || receivedStage != "Partitioning drive" {
		t.Fatalf("expected 50 and 'Partitioning drive', got %d, '%s'", receivedProgress, receivedStage)
	}
}
