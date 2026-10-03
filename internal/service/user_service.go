package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/port"
)

var validUsernameRegex = regexp.MustCompile(`^[a-zA-Z0-9._-]{3,32}$`)

// UserService manages local operator identities and role privileges.
type UserService struct {
	repo port.UserRepository
}

// NewUserService creates an initialized UserService.
func NewUserService(repo port.UserRepository) *UserService {
	return &UserService{repo: repo}
}

// EnsureDefaultAdmin verifies at least one administrator account exists, creating the default superuser if absent.
func (s *UserService) EnsureDefaultAdmin(ctx context.Context) error {
	count, err := s.repo.Count(ctx)
	if err != nil {
		return fmt.Errorf("failed checking user count: %w", err)
	}

	if count > 0 {
		return nil
	}

	slog.InfoContext(ctx, "no operator accounts discovered; provisioning default emergency administrator")

	defaultPassword := "admin" // Standard bootstrap password, prompt change on setup
	hash, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed hashing bootstrap admin password: %w", err)
	}

	adminUser := &domain.User{
		ID:          "usr-" + uuid.New().String(),
		Username:    "admin",
		DisplayName: "System Administrator",
		Email:       "admin@redwolf.internal",
		Role:        domain.RoleAdmin,
		Source:      domain.AuthSourceLocal,
		CreatedAt:   time.Now().UTC(),
	}

	if err := s.repo.Create(ctx, adminUser, string(hash)); err != nil {
		return fmt.Errorf("failed creating default administrator: %w", err)
	}

	slog.InfoContext(ctx, "default administrator account provisioned successfully",
		"username", adminUser.Username,
		"role", adminUser.Role,
	)

	return nil
}

// CreateUser registers a new local operator account.
func (s *UserService) CreateUser(ctx context.Context, req domain.CreateUserRequest) (*domain.User, error) {
	username := strings.TrimSpace(req.Username)
	if !validUsernameRegex.MatchString(username) {
		return nil, fmt.Errorf("invalid username '%s': must be 3-32 alphanumeric characters, dots, underscores, or hyphens", username)
	}

	if len(req.Password) < 8 {
		return nil, errors.New("password must be at least 8 characters long")
	}

	role := req.Role
	if role != domain.RoleAdmin && role != domain.RoleOperator && role != domain.RoleViewer {
		return nil, fmt.Errorf("invalid user role '%s': must be ADMIN, OPERATOR, or VIEWER", role)
	}

	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = username
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed hashing password: %w", err)
	}

	user := &domain.User{
		ID:          "usr-" + uuid.New().String(),
		Username:    username,
		DisplayName: displayName,
		Email:       strings.TrimSpace(req.Email),
		Role:        role,
		Source:      domain.AuthSourceLocal,
		CreatedAt:   time.Now().UTC(),
	}

	if err := s.repo.Create(ctx, user, string(hash)); err != nil {
		return nil, err
	}

	slog.InfoContext(ctx, "local user created",
		"user_id", user.ID,
		"username", user.Username,
		"role", user.Role,
	)

	return user, nil
}

// ListUsers retrieves all registered local operators.
func (s *UserService) ListUsers(ctx context.Context) ([]*domain.User, error) {
	return s.repo.List(ctx)
}

// GetUserByID retrieves a single user by ID.
func (s *UserService) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	return s.repo.GetByID(ctx, id)
}

// UpdateUser updates display name, email, and role for an existing account.
func (s *UserService) UpdateUser(ctx context.Context, id string, req domain.UpdateUserRequest) (*domain.User, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	role := req.Role
	if role != domain.RoleAdmin && role != domain.RoleOperator && role != domain.RoleViewer {
		return nil, fmt.Errorf("invalid user role '%s'", role)
	}

	// Safety check: if demoting an ADMIN, make sure at least one other ADMIN remains
	if existing.Role == domain.RoleAdmin && role != domain.RoleAdmin {
		adminCount, err := s.repo.CountAdmins(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed checking administrator count: %w", err)
		}
		if adminCount <= 1 {
			return nil, domain.ErrCannotDeleteLastAdmin
		}
	}

	displayName := strings.TrimSpace(req.DisplayName)
	if displayName != "" {
		existing.DisplayName = displayName
	}
	existing.Email = strings.TrimSpace(req.Email)
	existing.Role = role

	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
	}

	slog.InfoContext(ctx, "user account updated",
		"user_id", existing.ID,
		"username", existing.Username,
		"role", existing.Role,
	)

	return existing, nil
}

// ChangePassword updates the credentials for a user.
func (s *UserService) ChangePassword(ctx context.Context, id string, newPassword string) error {
	if len(newPassword) < 8 {
		return errors.New("new password must be at least 8 characters long")
	}

	// Ensure user exists
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed hashing password: %w", err)
	}

	if err := s.repo.UpdatePassword(ctx, id, string(hash)); err != nil {
		return err
	}

	slog.InfoContext(ctx, "user password changed", "user_id", id)
	return nil
}

// DeleteUser deletes an account, guarding against self-deletion and removal of the last administrator.
func (s *UserService) DeleteUser(ctx context.Context, id string, callerID string) error {
	if callerID != "" && id == callerID {
		return domain.ErrCannotDeleteSelf
	}

	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if user.Role == domain.RoleAdmin {
		adminCount, err := s.repo.CountAdmins(ctx)
		if err != nil {
			return fmt.Errorf("failed checking administrator count: %w", err)
		}
		if adminCount <= 1 {
			return domain.ErrCannotDeleteLastAdmin
		}
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	slog.InfoContext(ctx, "user deleted",
		"user_id", id,
		"username", user.Username,
	)

	return nil
}

// VerifyPassword validates the submitted plaintext password against the stored bcrypt hash.
func (s *UserService) VerifyPassword(ctx context.Context, username, password string) (*domain.User, error) {
	user, hash, err := s.repo.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil, domain.ErrInvalidCredentials
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	// Update last login timestamp asynchronously
	_ = s.repo.UpdateLastLogin(ctx, user.ID)
	user.LastLoginAt = time.Now().UTC()

	return user, nil
}
