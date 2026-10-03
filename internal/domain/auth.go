package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidCredentials   = errors.New("invalid username or password")
	ErrAccountLocked        = errors.New("account is locked")
	ErrUnauthorized         = errors.New("unauthorized")
	ErrDirectoryUnreachable = errors.New("directory service is unreachable")
	ErrUserNotFound         = errors.New("user not found")
	ErrUserExists           = errors.New("username already exists")
	ErrCannotDeleteLastAdmin = errors.New("cannot delete the last administrator")
	ErrCannotDeleteSelf     = errors.New("cannot delete own user account")
)

// UserRole defines privilege levels within the provisioning console.
type UserRole string

const (
	RoleAdmin    UserRole = "ADMIN"
	RoleOperator UserRole = "OPERATOR"
	RoleViewer   UserRole = "VIEWER"
)

// AuthSource identifies the identity provider.
type AuthSource string

const (
	AuthSourceLocal     AuthSource = "LOCAL"
	AuthSourceLDAP      AuthSource = "LDAP"
	AuthSourceAD        AuthSource = "ACTIVE_DIRECTORY"
	AuthSourceDirectory AuthSource = "DIRECTORY" // Unified Directory Service alias (LDAP/AD)
)

// User represents an authenticated operator identity.
type User struct {
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	DisplayName string     `json:"displayName"`
	Email       string     `json:"email"`
	Role        UserRole   `json:"role"`
	Source      AuthSource `json:"source"`
	CreatedAt   time.Time  `json:"createdAt"`
	LastLoginAt time.Time  `json:"lastLoginAt"`
}

// CreateUserRequest carries payload for creating a new local operator account.
type CreateUserRequest struct {
	Username    string   `json:"username"`
	DisplayName string   `json:"displayName"`
	Email       string   `json:"email"`
	Password    string   `json:"password"`
	Role        UserRole `json:"role"`
}

// UpdateUserRequest carries payload for editing operator metadata.
type UpdateUserRequest struct {
	DisplayName string   `json:"displayName"`
	Email       string   `json:"email"`
	Role        UserRole `json:"role"`
}

// ChangePasswordRequest carries password reset payload.
type ChangePasswordRequest struct {
	NewPassword string `json:"newPassword"`
}

// LoginRequest carries credentials submitted by the UI.
type LoginRequest struct {
	Username string     `json:"username"`
	Password string     `json:"password"`
	Source   AuthSource `json:"source"`
}

// LoginResponse returns the generated session token and user profile.
type LoginResponse struct {
	Token     string    `json:"token"`
	User      User      `json:"user"`
	ExpiresAt time.Time `json:"expiresAt"`
}

