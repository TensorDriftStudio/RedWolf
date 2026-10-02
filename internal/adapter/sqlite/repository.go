package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/port"
)

var _ port.NodeRepository = (*Repository)(nil)

// Repository implements port.NodeRepository backed by SQLite in WAL mode.
type Repository struct {
	db *sql.DB
}

// NewRepository initializes the SQLite database connection pool and schema.
func NewRepository(dbPath string) (*Repository, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Optimize connection pool for SQLite concurrency
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to apply database schema: %w", err)
	}

	return &Repository{db: db}, nil
}

// DB exposes the underlying database handle for auxiliary services.
func (r *Repository) DB() *sql.DB {
	return r.db
}

// Close closes the underlying database handle.
func (r *Repository) Close() error {
	return r.db.Close()
}

// Save inserts or updates a server node and its associated MAC address mappings.
func (r *Repository) Save(ctx context.Context, node *domain.ServerNode) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	cpuJSON, err := json.Marshal(node.CPU)
	if err != nil {
		return fmt.Errorf("failed to marshal cpu: %w", err)
	}

	memoryJSON, err := json.Marshal(node.Memory)
	if err != nil {
		return fmt.Errorf("failed to marshal memory: %w", err)
	}

	storageJSON, err := json.Marshal(node.Storage)
	if err != nil {
		return fmt.Errorf("failed to marshal storage: %w", err)
	}

	nicsJSON, err := json.Marshal(node.NICs)
	if err != nil {
		return fmt.Errorf("failed to marshal nics: %w", err)
	}

	bmcJSON, err := json.Marshal(node.BMC)
	if err != nil {
		return fmt.Errorf("failed to marshal bmc: %w", err)
	}

	var provJSON []byte
	if node.ProvisioningState != nil {
		provJSON, err = json.Marshal(node.ProvisioningState)
		if err != nil {
			return fmt.Errorf("failed to marshal provisioning state: %w", err)
		}
	}

	const upsertNodeSQL = `
	INSERT INTO nodes (
		id, vendor, model, serial_number, firmware_mode, bios_version,
		status, boot_mac, cpu_json, memory_json, storage_json, nics_json,
		bmc_json, provisioning_json, discovered_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		vendor = excluded.vendor,
		model = excluded.model,
		serial_number = excluded.serial_number,
		firmware_mode = excluded.firmware_mode,
		bios_version = excluded.bios_version,
		status = excluded.status,
		boot_mac = excluded.boot_mac,
		cpu_json = excluded.cpu_json,
		memory_json = excluded.memory_json,
		storage_json = excluded.storage_json,
		nics_json = excluded.nics_json,
		bmc_json = excluded.bmc_json,
		provisioning_json = excluded.provisioning_json,
		updated_at = excluded.updated_at;
	`

	now := time.Now().UTC()
	if node.DiscoveredAt.IsZero() {
		node.DiscoveredAt = now
	}
	node.UpdatedAt = now

	_, err = tx.ExecContext(ctx, upsertNodeSQL,
		node.ID,
		string(node.Vendor),
		node.Model,
		node.SerialNumber,
		string(node.FirmwareMode),
		node.BIOSVersion,
		string(node.Status),
		node.BootMAC(),
		string(cpuJSON),
		string(memoryJSON),
		string(storageJSON),
		string(nicsJSON),
		string(bmcJSON),
		string(provJSON),
		node.DiscoveredAt,
		node.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert node: %w", err)
	}

	// Refresh MAC mappings
	if _, err := tx.ExecContext(ctx, "DELETE FROM node_macs WHERE node_id = ?", node.ID); err != nil {
		return fmt.Errorf("failed to clear old mac mappings: %w", err)
	}

	for _, nic := range node.NICs {
		cleanMAC := strings.ToLower(strings.TrimSpace(nic.MAC))
		if cleanMAC != "" {
			_, err = tx.ExecContext(ctx, `
				INSERT INTO node_macs (mac, node_id) VALUES (?, ?)
				ON CONFLICT(mac) DO UPDATE SET node_id = excluded.node_id
			`, cleanMAC, node.ID)
			if err != nil {
				return fmt.Errorf("failed to insert mac mapping for %s: %w", cleanMAC, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit node transaction: %w", err)
	}

	return nil
}

// GetByID retrieves a single node by its primary UUID.
func (r *Repository) GetByID(ctx context.Context, id string) (*domain.ServerNode, error) {
	const querySQL = `
	SELECT id, vendor, model, serial_number, firmware_mode, bios_version,
	       status, cpu_json, memory_json, storage_json, nics_json,
	       bmc_json, provisioning_json, discovered_at, updated_at
	FROM nodes WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, querySQL, id)
	return r.scanNode(row)
}

// GetByMAC retrieves a server node by any of its recorded physical MAC addresses.
func (r *Repository) GetByMAC(ctx context.Context, mac string) (*domain.ServerNode, error) {
	cleanMAC := strings.ToLower(strings.TrimSpace(mac))
	const querySQL = `
	SELECT n.id, n.vendor, n.model, n.serial_number, n.firmware_mode, n.bios_version,
	       n.status, n.cpu_json, n.memory_json, n.storage_json, n.nics_json,
	       n.bmc_json, n.provisioning_json, n.discovered_at, n.updated_at
	FROM nodes n
	JOIN node_macs m ON n.id = m.node_id
	WHERE m.mac = ?
	`
	row := r.db.QueryRowContext(ctx, querySQL, cleanMAC)
	return r.scanNode(row)
}

// List returns all discovered nodes ordered by initial detection timestamp.
func (r *Repository) List(ctx context.Context) ([]*domain.ServerNode, error) {
	const querySQL = `
	SELECT id, vendor, model, serial_number, firmware_mode, bios_version,
	       status, cpu_json, memory_json, storage_json, nics_json,
	       bmc_json, provisioning_json, discovered_at, updated_at
	FROM nodes
	ORDER BY discovered_at DESC
	`
	rows, err := r.db.QueryContext(ctx, querySQL)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes list: %w", err)
	}
	defer rows.Close()

	var nodes []*domain.ServerNode
	for rows.Next() {
		node, err := r.scanNode(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during nodes iteration: %w", err)
	}

	return nodes, nil
}

// UpdateStatus changes the state of a node within an isolated transaction.
func (r *Repository) UpdateStatus(ctx context.Context, id string, status domain.NodeStatus) error {
	const updateSQL = `UPDATE nodes SET status = ?, updated_at = ? WHERE id = ?`
	res, err := r.db.ExecContext(ctx, updateSQL, string(status), time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("failed to update node status: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrNodeNotFound
	}
	return nil
}

// UpdateProvisioningState persists ongoing deployment telemetry.
func (r *Repository) UpdateProvisioningState(ctx context.Context, id string, state *domain.ProvisioningState) error {
	var provJSON []byte
	var err error
	if state != nil {
		provJSON, err = json.Marshal(state)
		if err != nil {
			return fmt.Errorf("failed to marshal provisioning state: %w", err)
		}
	}

	const updateSQL = `UPDATE nodes SET provisioning_json = ?, updated_at = ? WHERE id = ?`
	res, err := r.db.ExecContext(ctx, updateSQL, string(provJSON), time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("failed to update provisioning state: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrNodeNotFound
	}
	return nil
}

// Delete removes a node and cascades to its MAC mappings.
func (r *Repository) Delete(ctx context.Context, id string) error {
	const deleteSQL = `DELETE FROM nodes WHERE id = ?`
	res, err := r.db.ExecContext(ctx, deleteSQL, id)
	if err != nil {
		return fmt.Errorf("failed to delete node: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrNodeNotFound
	}
	return nil
}

// scanner abstracts both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func (r *Repository) scanNode(s scanner) (*domain.ServerNode, error) {
	var (
		node                                                      domain.ServerNode
		vendor, firmwareMode, status                              string
		cpuJSON, memJSON, storJSON, nicsJSON, bmcJSON, provJSON sql.NullString
	)

	err := s.Scan(
		&node.ID,
		&vendor,
		&node.Model,
		&node.SerialNumber,
		&firmwareMode,
		&node.BIOSVersion,
		&status,
		&cpuJSON,
		&memJSON,
		&storJSON,
		&nicsJSON,
		&bmcJSON,
		&provJSON,
		&node.DiscoveredAt,
		&node.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNodeNotFound
		}
		return nil, fmt.Errorf("failed to scan node row: %w", err)
	}

	node.Vendor = domain.Vendor(vendor)
	node.FirmwareMode = domain.FirmwareMode(firmwareMode)
	node.Status = domain.NodeStatus(status)

	if cpuJSON.Valid && cpuJSON.String != "" {
		_ = json.Unmarshal([]byte(cpuJSON.String), &node.CPU)
	}
	if memJSON.Valid && memJSON.String != "" {
		_ = json.Unmarshal([]byte(memJSON.String), &node.Memory)
	}
	if storJSON.Valid && storJSON.String != "" {
		_ = json.Unmarshal([]byte(storJSON.String), &node.Storage)
	}
	if nicsJSON.Valid && nicsJSON.String != "" {
		_ = json.Unmarshal([]byte(nicsJSON.String), &node.NICs)
	}
	if bmcJSON.Valid && bmcJSON.String != "" {
		_ = json.Unmarshal([]byte(bmcJSON.String), &node.BMC)
	}
	if provJSON.Valid && provJSON.String != "" {
		var prov domain.ProvisioningState
		if err := json.Unmarshal([]byte(provJSON.String), &prov); err == nil {
			node.ProvisioningState = &prov
		}
	}

	return &node, nil
}
