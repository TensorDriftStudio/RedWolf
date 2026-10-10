package redfish

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

func TestClient_GetPowerState(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/redfish/v1/Systems/1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"PowerState": "On",
		})
	})

	server := httptest.NewTLSServer(mux)
	defer server.Close()

	client := NewClient()
	host := strings.TrimPrefix(server.URL, "https://")

	ctx := context.Background()
	state, err := client.GetPowerState(ctx, domain.BMCInfo{IP: host}, &domain.BMCCredential{
		Username: "ADMIN",
		Password: "ADMIN",
	})
	if err != nil {
		t.Fatalf("expected power state query to succeed: %v", err)
	}
	if state != domain.PowerStateOn {
		t.Fatalf("expected PowerStateOn, got %s", state)
	}
}

func TestClient_ExecutePowerAction(t *testing.T) {
	var receivedResetType string
	mux := http.NewServeMux()
	mux.HandleFunc("/redfish/v1/Systems/1/Actions/ComputerSystem.Reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload struct {
			ResetType string `json:"ResetType"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		receivedResetType = payload.ResetType
		w.WriteHeader(http.StatusNoContent)
	})

	server := httptest.NewTLSServer(mux)
	defer server.Close()

	client := NewClient()
	host := strings.TrimPrefix(server.URL, "https://")

	ctx := context.Background()
	err := client.ExecutePowerAction(ctx, domain.BMCInfo{IP: host}, nil, domain.PowerActionOn)
	if err != nil {
		t.Fatalf("expected execute power action to succeed: %v", err)
	}
	if receivedResetType != "On" {
		t.Fatalf("expected ResetType 'On', got '%s'", receivedResetType)
	}
}

func TestClient_UpdateCredentials(t *testing.T) {
	var receivedPassword string
	mux := http.NewServeMux()
	mux.HandleFunc("/redfish/v1/AccountService/Accounts/2", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload struct {
			Password string `json:"Password"`
			Enabled  bool   `json:"Enabled"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		receivedPassword = payload.Password
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewTLSServer(mux)
	defer server.Close()

	client := NewClient()
	host := strings.TrimPrefix(server.URL, "https://")

	ctx := context.Background()
	err := client.UpdateCredentials(ctx, domain.BMCInfo{IP: host}, nil, "ADMIN", "NewSecur3P@ss!", 2)
	if err != nil {
		t.Fatalf("expected credential update to succeed: %v", err)
	}
	if receivedPassword != "NewSecur3P@ss!" {
		t.Fatalf("expected password 'NewSecur3P@ss!', got '%s'", receivedPassword)
	}
}
