package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/port"
)

var _ port.UserRepository = (*UserRepository)(nil)

// UserRepository implements port.UserRepository using SQLite.
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository creates an initialized SQLite user repository.
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create inserts a new user record with password hash.
func (r *UserRepository) Create(ctx context.Context, user *domain.User, passwordHash string) error {
	query := `INSERT INTO users (
		id, username, password_hash, display_name, email, role, source, created_at, updated_at, last_login_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	var lastLogin sql.NullTime
	if !user.LastLoginAt.IsZero() {
		lastLogin = sql.NullTime{Time: user.LastLoginAt.UTC(), Valid: true}
	}

	_, err := r.db.ExecContext(ctx, query,
		user.ID,
		user.Username,
		passwordHash,
		user.DisplayName,
		user.Email,
		string(user.Role),
		string(user.Source),
		user.CreatedAt.UTC(),
		time.Now().UTC(),
		lastLogin,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: users.username") {
			return domain.ErrUserExists
		}
		return fmt.Errorf("failed inserting user: %w", err)
	}

	return nil
}

// GetByID retrieves a user by ID.
func (r *UserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	query := `SELECT id, username, display_name, email, role, source, created_at, last_login_at 
		FROM users WHERE id = ?`

	var u domain.User
	var roleStr, sourceStr string
	var lastLogin sql.NullTime

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&u.ID,
		&u.Username,
		&u.DisplayName,
		&u.Email,
		&roleStr,
		&sourceStr,
		&u.CreatedAt,
		&lastLogin,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed querying user by id: %w", err)
	}

	u.Role = domain.UserRole(roleStr)
	u.Source = domain.AuthSource(sourceStr)
	if lastLogin.Valid {
		u.LastLoginAt = lastLogin.Time
	}

	return &u, nil
}

// GetByUsername retrieves a user and their password hash by username.
func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*domain.User, string, error) {
	query := `SELECT id, username, password_hash, display_name, email, role, source, created_at, last_login_at 
		FROM users WHERE username = ?`

	var u domain.User
	var passwordHash, roleStr, sourceStr string
	var lastLogin sql.NullTime

	err := r.db.QueryRowContext(ctx, query, username).Scan(
		&u.ID,
		&u.Username,
		&passwordHash,
		&u.DisplayName,
		&u.Email,
		&roleStr,
		&sourceStr,
		&u.CreatedAt,
		&lastLogin,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", domain.ErrUserNotFound
		}
		return nil, "", fmt.Errorf("failed querying user by username: %w", err)
	}

	u.Role = domain.UserRole(roleStr)
	u.Source = domain.AuthSource(sourceStr)
	if lastLogin.Valid {
		u.LastLoginAt = lastLogin.Time
	}

	return &u, passwordHash, nil
}

// List returns all registered local users.
func (r *UserRepository) List(ctx context.Context) ([]*domain.User, error) {
	query := `SELECT id, username, display_name, email, role, source, created_at, last_login_at 
		FROM users ORDER BY created_at ASC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed listing users: %w", err)
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		var u domain.User
		var roleStr, sourceStr string
		var lastLogin sql.NullTime

		if err := rows.Scan(
			&u.ID,
			&u.Username,
			&u.DisplayName,
			&u.Email,
			&roleStr,
			&sourceStr,
			&u.CreatedAt,
			&lastLogin,
		); err != nil {
			return nil, fmt.Errorf("failed scanning user row: %w", err)
		}

		u.Role = domain.UserRole(roleStr)
		u.Source = domain.AuthSource(sourceStr)
		if lastLogin.Valid {
			u.LastLoginAt = lastLogin.Time
		}

		users = append(users, &u)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("users iteration error: %w", err)
	}

	return users, nil
}

// Update modifies display name, email, and role for an existing user.
func (r *UserRepository) Update(ctx context.Context, user *domain.User) error {
	query := `UPDATE users SET display_name = ?, email = ?, role = ?, updated_at = ? WHERE id = ?`

	res, err := r.db.ExecContext(ctx, query,
		user.DisplayName,
		user.Email,
		string(user.Role),
		time.Now().UTC(),
		user.ID,
	)
	if err != nil {
		return fmt.Errorf("failed updating user: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed checking rows affected: %w", err)
	}
	if affected == 0 {
		return domain.ErrUserNotFound
	}

	return nil
}

// UpdatePassword updates the stored password hash for a user.
func (r *UserRepository) UpdatePassword(ctx context.Context, id string, passwordHash string) error {
	query := `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`

	res, err := r.db.ExecContext(ctx, query, passwordHash, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("failed updating user password: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed checking rows affected: %w", err)
	}
	if affected == 0 {
		return domain.ErrUserNotFound
	}

	return nil
}

// UpdateLastLogin updates the last login timestamp for a user.
func (r *UserRepository) UpdateLastLogin(ctx context.Context, id string) error {
	query := `UPDATE users SET last_login_at = ? WHERE id = ?`

	_, err := r.db.ExecContext(ctx, query, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("failed updating last login: %w", err)
	}

	return nil
}

// Delete removes a user record by ID.
func (r *UserRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM users WHERE id = ?`

	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed deleting user: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed checking rows affected: %w", err)
	}
	if affected == 0 {
		return domain.ErrUserNotFound
	}

	return nil
}

// Count returns the total number of users.
func (r *UserRepository) Count(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed counting users: %w", err)
	}
	return count, nil
}

// CountAdmins returns the count of users with the ADMIN role.
func (r *UserRepository) CountAdmins(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE role = ?", string(domain.RoleAdmin)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed counting admins: %w", err)
	}
	return count, nil
}
