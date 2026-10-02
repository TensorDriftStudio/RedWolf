package domain_test

import (
	"testing"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

func TestPowerAction_Validate(t *testing.T) {
	validActions := []domain.PowerAction{
		domain.PowerActionOn,
		domain.PowerActionOff,
		domain.PowerActionReset,
		domain.PowerActionGracefulShutdown,
		domain.PowerActionPXEReboot,
	}

	for _, a := range validActions {
		if err := a.Validate(); err != nil {
			t.Errorf("expected valid power action %s, got error: %v", a, err)
		}
	}

	invalidAction := domain.PowerAction("invalid_action_foo")
	if err := invalidAction.Validate(); err == nil {
		t.Errorf("expected error for invalid action, got nil")
	}
}
