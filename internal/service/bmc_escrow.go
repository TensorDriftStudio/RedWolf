package service

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/tensordriftstudio/redwolf/internal/adapter/crypto"
	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/port"
)

// BMCEscrowService coordinates out-of-band management credential vaulting and rotation.
type BMCEscrowService struct {
	db        *sql.DB
	vaultKey  string
	repo      port.NodeRepository
	events    port.EventBroadcaster
}

// NewBMCEscrowService creates an initialized BMC escrow service.
func NewBMCEscrowService(db *sql.DB, vaultKey string, repo port.NodeRepository, events port.EventBroadcaster) *BMCEscrowService {
	svc := &BMCEscrowService{
		db:       db,
		vaultKey: vaultKey,
		repo:     repo,
		events:   events,
	}
	svc.initSchema()
	return svc
}

func (s *BMCEscrowService) initSchema() {
	if s.db == nil {
		return
	}
	query := `CREATE TABLE IF NOT EXISTS bmc_credentials (
		node_id TEXT PRIMARY KEY,
		username TEXT NOT NULL,
		encrypted_password TEXT NOT NULL,
		user_slot INTEGER NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (node_id) REFERENCES server_nodes(id) ON DELETE CASCADE
	);`
	_, _ = s.db.Exec(query)
}

// RotateCredentials generates a safe 14-16 character password and escrows it in the AES-256 vault.
func (s *BMCEscrowService) RotateCredentials(ctx context.Context, nodeID string, targetUsername string, slot int) (*domain.BMCCredential, error) {
	node, err := s.repo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}

	if targetUsername == "" {
		targetUsername = "redwolf"
		if node.Vendor == domain.VendorSupermicro {
			targetUsername = "ADMIN"
		}
	}
	if slot <= 0 {
		slot = 2 // Standard IPMI 2.0 administrator slot
	}

	// Generate 15-character RFC compliant password
	newPassword := domain.GenerateUniversalSafePassword()

	// Encrypt using AES-256-GCM
	encrypted, err := crypto.VaultEncrypt(s.vaultKey, []byte(newPassword))
	if err != nil {
		return nil, fmt.Errorf("encryption error: %w", err)
	}

	now := time.Now().UTC()
	cred := &domain.BMCCredential{
		NodeID:            nodeID,
		Username:          targetUsername,
		Password:          newPassword,
		EncryptedPassword: encrypted,
		UserSlot:          slot,
		UpdatedAt:         now,
	}

	if s.db != nil {
		query := `INSERT INTO bmc_credentials (node_id, username, encrypted_password, user_slot, updated_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(node_id) DO UPDATE SET 
				username = excluded.username,
				encrypted_password = excluded.encrypted_password,
				user_slot = excluded.user_slot,
				updated_at = excluded.updated_at`
		_, err := s.db.ExecContext(ctx, query, cred.NodeID, cred.Username, cred.EncryptedPassword, cred.UserSlot, cred.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed persisting encrypted credentials: %w", err)
		}
	}

	// Update node status
	node.BMC.CredentialsUpdated = true
	_ = s.repo.Save(ctx, node)
	s.events.Publish("node:updated", node)

	slog.InfoContext(ctx, "bmc credentials rotated and escrowed in vault",
		"node_id", nodeID,
		"vendor", node.Vendor,
		"username", cred.Username,
		"slot", cred.UserSlot,
		"length", len(newPassword),
	)

	return cred, nil
}

// GetCredentials retrieves and decrypts the vaulted BMC credentials for authorized operators.
func (s *BMCEscrowService) GetCredentials(ctx context.Context, nodeID string) (*domain.BMCCredential, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var username, encrypted string
	var slot int
	var updatedAt time.Time

	query := `SELECT username, encrypted_password, user_slot, updated_at FROM bmc_credentials WHERE node_id = ?`
	err := s.db.QueryRowContext(ctx, query, nodeID).Scan(&username, &encrypted, &slot, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNodeNotFound
		}
		return nil, fmt.Errorf("failed querying bmc credentials: %w", err)
	}

	decryptedBytes, err := crypto.VaultDecrypt(s.vaultKey, encrypted)
	if err != nil {
		return nil, fmt.Errorf("failed decrypting credentials: %w", err)
	}

	return &domain.BMCCredential{
		NodeID:    nodeID,
		Username:  username,
		Password:  string(decryptedBytes),
		UserSlot:  slot,
		UpdatedAt: updatedAt,
	}, nil
}
