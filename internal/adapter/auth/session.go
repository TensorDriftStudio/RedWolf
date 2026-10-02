package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// Session represents an active authenticated user session.
type Session struct {
	Token     string
	User      domain.User
	ExpiresAt time.Time
}

// SessionManager manages active user sessions in memory.
type SessionManager struct {
	sessions sync.Map // map[string]*Session
	ttl      time.Duration
}

// NewSessionManager creates a token session manager with specified TTL.
func NewSessionManager(ttl time.Duration) *SessionManager {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &SessionManager{
		ttl: ttl,
	}
}

// CreateSession generates a secure token and records the active session.
func (m *SessionManager) CreateSession(user domain.User) (*Session, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(tokenBytes)

	now := time.Now().UTC()
	session := &Session{
		Token:     token,
		User:      user,
		ExpiresAt: now.Add(m.ttl),
	}

	m.sessions.Store(token, session)
	return session, nil
}

// ValidateSession verifies if a bearer token exists and has not expired.
func (m *SessionManager) ValidateSession(token string) (*domain.User, bool) {
	val, ok := m.sessions.Load(token)
	if !ok {
		return nil, false
	}
	session := val.(*Session)

	if time.Now().UTC().After(session.ExpiresAt) {
		m.sessions.Delete(token)
		return nil, false
	}

	return &session.User, true
}

// InvalidateSession removes a token on logout.
func (m *SessionManager) InvalidateSession(token string) {
	m.sessions.Delete(token)
}
