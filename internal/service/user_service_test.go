package service

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
	"github.com/tensordriftstudio/redwolf/internal/adapter/sqlite"
	"github.com/tensordriftstudio/redwolf/internal/domain"
)

func setupTestUserSvc(t *testing.T) (*sql.DB, *UserService) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed opening in-memory db: %v", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		display_name TEXT NOT NULL,
		email TEXT NOT NULL,
		role TEXT NOT NULL,
		source TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		last_login_at TIMESTAMP
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed executing schema: %v", err)
	}

	repo := sqlite.NewUserRepository(db)
	svc := NewUserService(repo)
	return db, svc
}

func TestUserService_BootstrapAndAuth(t *testing.T) {
	ctx := context.Background()
	db, svc := setupTestUserSvc(t)
	defer db.Close()

	// 1. EnsureDefaultAdmin seeds 'admin'
	if err := svc.EnsureDefaultAdmin(ctx); err != nil {
		t.Fatalf("failed EnsureDefaultAdmin: %v", err)
	}

	// 2. Calling EnsureDefaultAdmin again is idempotent
	if err := svc.EnsureDefaultAdmin(ctx); err != nil {
		t.Fatalf("idempotent EnsureDefaultAdmin failed: %v", err)
	}

	// 3. Authenticate as admin
	admin, err := svc.VerifyPassword(ctx, "admin", "admin")
	if err != nil {
		t.Fatalf("failed verifying admin password: %v", err)
	}
	if admin.Username != "admin" || admin.Role != domain.RoleAdmin {
		t.Fatalf("unexpected admin user: %+v", admin)
	}

	// Invalid password
	_, err = svc.VerifyPassword(ctx, "admin", "wrongpassword")
	if err != domain.ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got: %v", err)
	}

	// 4. Create operator account
	op, err := svc.CreateUser(ctx, domain.CreateUserRequest{
		Username:    "operator1",
		DisplayName: "Data Center Operator",
		Email:       "op@datacenter.net",
		Password:    "SecureOperatorPass123!",
		Role:        domain.RoleOperator,
	})
	if err != nil {
		t.Fatalf("failed creating operator: %v", err)
	}
	if op.Username != "operator1" || op.Role != domain.RoleOperator {
		t.Fatalf("unexpected operator: %+v", op)
	}

	// 5. Authenticate as operator
	verifiedOp, err := svc.VerifyPassword(ctx, "operator1", "SecureOperatorPass123!")
	if err != nil {
		t.Fatalf("failed verifying operator password: %v", err)
	}
	if verifiedOp.ID != op.ID {
		t.Fatalf("expected ID %s, got %s", op.ID, verifiedOp.ID)
	}

	// 6. Delete self guard
	err = svc.DeleteUser(ctx, admin.ID, admin.ID)
	if err != domain.ErrCannotDeleteSelf {
		t.Fatalf("expected ErrCannotDeleteSelf, got %v", err)
	}

	// 7. Delete last admin guard
	err = svc.DeleteUser(ctx, admin.ID, "some-other-caller")
	if err != domain.ErrCannotDeleteLastAdmin {
		t.Fatalf("expected ErrCannotDeleteLastAdmin, got %v", err)
	}

	// 8. Delete operator succeeds
	if err := svc.DeleteUser(ctx, op.ID, admin.ID); err != nil {
		t.Fatalf("failed deleting operator: %v", err)
	}

	// 9. Validation tests
	_, err = svc.CreateUser(ctx, domain.CreateUserRequest{
		Username: "ab", // too short
		Password: "password123",
		Role:     domain.RoleOperator,
	})
	if err == nil {
		t.Fatalf("expected error for short username")
	}

	_, err = svc.CreateUser(ctx, domain.CreateUserRequest{
		Username: "valid_name",
		Password: "shrt", // too short
		Role:     domain.RoleOperator,
	})
	if err == nil {
		t.Fatalf("expected error for short password")
	}
}
