package port

import (
	"context"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// DNSMasqManager defines the interface for supervising the network boot daemon.
type DNSMasqManager interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Reload(ctx context.Context) error
	RenderConfig(ctx context.Context) error
	UpdateNetworkSettings(ctx context.Context, netCfg domain.NetworkSettings, iface string) error
}
