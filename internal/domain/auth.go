package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrAccountLocked      = errors.New("account is locked")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrDirectoryUnreachable = errors.New("directory service is unreachable")
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
	AuthSourceLocal AuthSource = "LOCAL"
	AuthSourceLDAP  AuthSource = "LDAP"
	AuthSourceAD    AuthSource = "ACTIVE_DIRECTORY"
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
