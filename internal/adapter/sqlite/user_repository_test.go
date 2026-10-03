package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
	"github.com/tensordriftstudio/redwolf/internal/domain"
)

func setupTestUserDB(t *testing.T) (*sql.DB, *UserRepository) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed opening in-memory db: %v", err)
	}

	if _, err := db.Exec(schemaSQL); err != nil {
		t.Fatalf("failed executing schema: %v", err)
	}

	return db, NewUserRepository(db)
}

func TestUserRepository_CRUD(t *testing.T) {
	ctx := context.Background()
	db, repo := setupTestUserDB(t)
	defer db.Close()

	// 1. Initial count
	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("unexpected error on Count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected count 0, got %d", count)
	}

	// 2. Create user
	user := &domain.User{
		ID:          "usr-test-1",
		Username:    "johndoe",
		DisplayName: "John Doe",
		Email:       "johndoe@example.com",
		Role:        domain.RoleOperator,
		Source:      domain.AuthSourceLocal,
		CreatedAt:   time.Now().UTC(),
	}

	if err := repo.Create(ctx, user, "$2a$10$hashedteststring"); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// 3. Duplicate username should return ErrUserExists
	dup := &domain.User{
		ID:          "usr-test-2",
		Username:    "johndoe",
		DisplayName: "Another John",
		Role:        domain.RoleViewer,
		Source:      domain.AuthSourceLocal,
		CreatedAt:   time.Now().UTC(),
	}
	if err := repo.Create(ctx, dup, "hash"); err != domain.ErrUserExists {
		t.Fatalf("expected ErrUserExists, got %v", err)
	}

	// 4. Get by ID
	fetched, err := repo.GetByID(ctx, "usr-test-1")
	if err != nil {
		t.Fatalf("failed to get user by id: %v", err)
	}
	if fetched.Username != "johndoe" || fetched.DisplayName != "John Doe" {
		t.Fatalf("unexpected fetched user: %+v", fetched)
	}

	// 5. Get by Username
	byUser, hash, err := repo.GetByUsername(ctx, "johndoe")
	if err != nil {
		t.Fatalf("failed to get user by username: %v", err)
	}
	if byUser.ID != "usr-test-1" || hash != "$2a$10$hashedteststring" {
		t.Fatalf("unexpected user or hash: %+v, hash: %s", byUser, hash)
	}

	// 6. Update user
	fetched.DisplayName = "Johnathan Doe"
	fetched.Role = domain.RoleAdmin
	if err := repo.Update(ctx, fetched); err != nil {
		t.Fatalf("failed to update user: %v", err)
	}

	updated, err := repo.GetByID(ctx, "usr-test-1")
	if err != nil {
		t.Fatalf("failed to get updated user: %v", err)
	}
	if updated.DisplayName != "Johnathan Doe" || updated.Role != domain.RoleAdmin {
		t.Fatalf("user was not updated correctly: %+v", updated)
	}

	// 7. Count admins
	adminCount, err := repo.CountAdmins(ctx)
	if err != nil {
		t.Fatalf("failed counting admins: %v", err)
	}
	if adminCount != 1 {
		t.Fatalf("expected 1 admin, got %d", adminCount)
	}

	// 8. Update password
	if err := repo.UpdatePassword(ctx, "usr-test-1", "$2a$10$newhash"); err != nil {
		t.Fatalf("failed to update password: %v", err)
	}
	_, newHash, err := repo.GetByUsername(ctx, "johndoe")
	if err != nil || newHash != "$2a$10$newhash" {
		t.Fatalf("expected new hash, got %s, err: %v", newHash, err)
	}

	// 9. Update last login
	if err := repo.UpdateLastLogin(ctx, "usr-test-1"); err != nil {
		t.Fatalf("failed to update last login: %v", err)
	}
	withLogin, err := repo.GetByID(ctx, "usr-test-1")
	if err != nil || withLogin.LastLoginAt.IsZero() {
		t.Fatalf("expected non-zero last login, got %+v", withLogin)
	}

	// 10. List users
	users, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("failed listing users: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user in list, got %d", len(users))
	}

	// 11. Delete user
	if err := repo.Delete(ctx, "usr-test-1"); err != nil {
		t.Fatalf("failed deleting user: %v", err)
	}

	_, err = repo.GetByID(ctx, "usr-test-1")
	if err != domain.ErrUserNotFound {
		t.Fatalf("expected ErrUserNotFound after delete, got %v", err)
	}
}
