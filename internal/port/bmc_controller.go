package port

import (
	"context"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// BMCController defines out-of-band management operations (Redfish and IPMI 2.0).
type BMCController interface {
	GetPowerState(ctx context.Context, bmc domain.BMCInfo, creds *domain.BMCCredential) (domain.PowerState, error)
	ExecutePowerAction(ctx context.Context, bmc domain.BMCInfo, creds *domain.BMCCredential, action domain.PowerAction) error
	SetOneTimePXEBoot(ctx context.Context, bmc domain.BMCInfo, creds *domain.BMCCredential) error
	UpdateCredentials(ctx context.Context, bmc domain.BMCInfo, currentCreds *domain.BMCCredential, newUsername, newPassword string, slot int) error
}
