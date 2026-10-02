package domain

import "testing"

func TestServerNode_FSM_Transitions(t *testing.T) {
	node := &ServerNode{
		ID:     "test-fsm",
		Status: NodeStatusDiscovering,
		NICs: []NetworkInterface{
			{Name: "eth0", MAC: "00:11:22:33:44:55", IsBoot: false},
			{Name: "eth1", MAC: "aa:bb:cc:dd:ee:ff", IsBoot: true},
		},
	}

	// Test BootMAC identification
	if mac := node.BootMAC(); mac != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("expected boot mac aa:bb:cc:dd:ee:ff, got %s", mac)
	}

	// 1. DISCOVERING -> READY (valid)
	if err := node.TransitionTo(NodeStatusReady); err != nil {
		t.Fatalf("unexpected error transitioning to READY: %v", err)
	}

	// 2. READY -> ACTIVE directly (invalid without provisioning)
	if err := node.TransitionTo(NodeStatusActive); err == nil {
		t.Fatalf("expected error transitioning directly from READY to ACTIVE, got nil")
	}

	// 3. READY -> PROVISIONING (valid)
	if err := node.TransitionTo(NodeStatusProvisioning); err != nil {
		t.Fatalf("unexpected error transitioning to PROVISIONING: %v", err)
	}

	// 4. PROVISIONING -> ACTIVE (valid)
	if err := node.TransitionTo(NodeStatusActive); err != nil {
		t.Fatalf("unexpected error transitioning to ACTIVE: %v", err)
	}
}
