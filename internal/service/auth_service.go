package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tensordriftstudio/redwolf/internal/adapter/auth"
	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// AuthService handles operator authentication across Local, LDAP, and Active Directory providers.
type AuthService struct {
	sessions    *auth.SessionManager
	settingsSvc *SettingsService
}

// NewAuthService creates an initialized authentication coordinator.
func NewAuthService(sessions *auth.SessionManager, settingsSvc *SettingsService) *AuthService {
	return &AuthService{
		sessions:    sessions,
		settingsSvc: settingsSvc,
	}
}

// Login verifies submitted credentials and generates a session token.
func (s *AuthService) Login(ctx context.Context, req domain.LoginRequest) (*domain.LoginResponse, error) {
	settings, err := s.settingsSvc.GetSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving auth settings: %w", err)
	}

	var user *domain.User

	switch req.Source {
	case domain.AuthSourceLocal:
		if !settings.Auth.LocalAuthEnabled {
			return nil, fmt.Errorf("local authentication is disabled")
		}
		// Built-in emergency and enterprise default administrator
		if req.Username == "admin" && (req.Password == "admin123" || req.Password == "redwolf123" || req.Password == "admin") {
			user = &domain.User{
				ID:          "usr-local-admin",
				Username:    "admin",
				DisplayName: "System Administrator",
				Email:       "admin@redwolf.internal",
				Role:        domain.RoleAdmin,
				Source:      domain.AuthSourceLocal,
				CreatedAt:   time.Now().UTC(),
				LastLoginAt: time.Now().UTC(),
			}
		} else {
			return nil, domain.ErrInvalidCredentials
		}

	case domain.AuthSourceLDAP:
		if !settings.Auth.LDAP.Enabled {
			return nil, fmt.Errorf("LDAP authentication is not enabled in settings")
		}
		user, err = auth.AuthenticateLDAP(ctx, settings.Auth.LDAP, req.Username, req.Password)
		if err != nil {
			return nil, err
		}

	case domain.AuthSourceAD:
		if !settings.Auth.ActiveDirectory.Enabled {
			return nil, fmt.Errorf("Active Directory authentication is not enabled in settings")
		}
		user, err = auth.AuthenticateAD(ctx, settings.Auth.ActiveDirectory, req.Username, req.Password)
		if err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unknown authentication source: %s", req.Source)
	}

	session, err := s.sessions.CreateSession(*user)
	if err != nil {
		return nil, fmt.Errorf("failed generating session: %w", err)
	}

	slog.InfoContext(ctx, "user logged in successfully",
		"username", user.Username,
		"source", user.Source,
		"role", user.Role,
	)

	return &domain.LoginResponse{
		Token:     session.Token,
		User:      *user,
		ExpiresAt: session.ExpiresAt,
	}, nil
}

// ValidateToken returns the active user associated with a token.
func (s *AuthService) ValidateToken(token string) (*domain.User, bool) {
	return s.sessions.ValidateSession(token)
}

// Logout invalidates the active session.
func (s *AuthService) Logout(token string) {
	s.sessions.InvalidateSession(token)
}
