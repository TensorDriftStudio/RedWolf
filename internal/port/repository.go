package port

import (
	"context"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// NodeRepository defines the storage abstraction for discovered server nodes.
type NodeRepository interface {
	Save(ctx context.Context, node *domain.ServerNode) error
	GetByID(ctx context.Context, id string) (*domain.ServerNode, error)
	GetByMAC(ctx context.Context, mac string) (*domain.ServerNode, error)
	List(ctx context.Context) ([]*domain.ServerNode, error)
	UpdateStatus(ctx context.Context, id string, status domain.NodeStatus) error
	UpdateProvisioningState(ctx context.Context, id string, state *domain.ProvisioningState) error
	Delete(ctx context.Context, id string) error
}
