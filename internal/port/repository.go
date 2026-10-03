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

// UserRepository defines the storage abstraction for local user accounts and authentication.
type UserRepository interface {
	Create(ctx context.Context, user *domain.User, passwordHash string) error
	GetByID(ctx context.Context, id string) (*domain.User, error)
	GetByUsername(ctx context.Context, username string) (*domain.User, string, error)
	List(ctx context.Context) ([]*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
	UpdatePassword(ctx context.Context, id string, passwordHash string) error
	UpdateLastLogin(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
	Count(ctx context.Context) (int, error)
	CountAdmins(ctx context.Context) (int, error)
}

