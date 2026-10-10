package ipmi

import (
	"context"
	"reflect"
	"testing"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

func TestRemoteController_buildBaseArgs(t *testing.T) {
	c := NewRemoteController()

	// Default fallback ADMIN/ADMIN
	argsDefault := c.buildBaseArgs("192.168.1.100", nil)
	expectedDefault := []string{"-I", "lanplus", "-H", "192.168.1.100", "-U", "ADMIN", "-P", "ADMIN"}
	if !reflect.DeepEqual(argsDefault, expectedDefault) {
		t.Fatalf("expected %v, got %v", expectedDefault, argsDefault)
	}

	// Custom credentials
	argsCustom := c.buildBaseArgs("10.0.0.50", &domain.BMCCredential{
		Username: "operator",
		Password: "CustomSecretPassword123!",
	})
	expectedCustom := []string{"-I", "lanplus", "-H", "10.0.0.50", "-U", "operator", "-P", "CustomSecretPassword123!"}
	if !reflect.DeepEqual(argsCustom, expectedCustom) {
		t.Fatalf("expected %v, got %v", expectedCustom, argsCustom)
	}
}

func TestRemoteController_EmptyIPValidation(t *testing.T) {
	c := NewRemoteController()
	ctx := context.Background()

	_, err := c.GetPowerState(ctx, domain.BMCInfo{IP: ""}, nil)
	if err == nil {
		t.Fatal("expected error on empty IP for GetPowerState")
	}

	err = c.ExecutePowerAction(ctx, domain.BMCInfo{IP: ""}, nil, domain.PowerActionOn)
	if err == nil {
		t.Fatal("expected error on empty IP for ExecutePowerAction")
	}

	err = c.SetOneTimePXEBoot(ctx, domain.BMCInfo{IP: ""}, nil)
	if err == nil {
		t.Fatal("expected error on empty IP for SetOneTimePXEBoot")
	}

	err = c.UpdateCredentials(ctx, domain.BMCInfo{IP: ""}, nil, "ADMIN", "pass", 2)
	if err == nil {
		t.Fatal("expected error on empty IP for UpdateCredentials")
	}
}

func TestRemoteController_UnsupportedPowerAction(t *testing.T) {
	c := NewRemoteController()
	ctx := context.Background()

	err := c.ExecutePowerAction(ctx, domain.BMCInfo{IP: "192.168.1.100"}, nil, domain.PowerAction("invalid_action"))
	if err == nil {
		t.Fatal("expected error for unsupported power action")
	}
}
