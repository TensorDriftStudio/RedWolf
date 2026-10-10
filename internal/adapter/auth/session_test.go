package auth

import (
	"testing"
	"time"

	"github.com/tensordriftstudio/redwolf/internal/domain"
)

func TestSessionManager_Lifecycle(t *testing.T) {
	mgr := NewSessionManager(50 * time.Millisecond)

	user := domain.User{
		ID:       "usr-1",
		Username: "admin",
		Role:     domain.RoleAdmin,
	}

	// 1. Create Session
	session, err := mgr.CreateSession(user)
	if err != nil {
		t.Fatalf("failed creating session: %v", err)
	}
	if session.Token == "" {
		t.Fatal("expected non-empty token")
	}

	// 2. Validate Session (Active)
	retrieved, valid := mgr.ValidateSession(session.Token)
	if !valid || retrieved == nil {
		t.Fatal("expected session to be valid")
	}
	if retrieved.ID != "usr-1" {
		t.Fatalf("expected usr-1, got %s", retrieved.ID)
	}

	// 3. Expiration
	time.Sleep(70 * time.Millisecond)
	expiredUser, valid := mgr.ValidateSession(session.Token)
	if valid || expiredUser != nil {
		t.Fatal("expected session to be expired")
	}

	// 4. Invalidation / Logout
	newSession, err := mgr.CreateSession(user)
	if err != nil {
		t.Fatalf("failed creating second session: %v", err)
	}
	mgr.InvalidateSession(newSession.Token)
	_, valid = mgr.ValidateSession(newSession.Token)
	if valid {
		t.Fatal("expected invalidated session to be invalid")
	}
}
