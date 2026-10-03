package http

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "modernc.org/sqlite"
	adapterAuth "github.com/tensordriftstudio/redwolf/internal/adapter/auth"
	adapterSQLite "github.com/tensordriftstudio/redwolf/internal/adapter/sqlite"
	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/service"
)

func setupTestUserHTTP(t *testing.T) (http.Handler, *service.UserService, *service.AuthService) {
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
	CREATE TABLE IF NOT EXISTS system_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed executing schema: %v", err)
	}

	userRepo := adapterSQLite.NewUserRepository(db)
	userSvc := service.NewUserService(userRepo)
	if err := userSvc.EnsureDefaultAdmin(context.Background()); err != nil {
		t.Fatalf("failed EnsureDefaultAdmin: %v", err)
	}

	settingsSvc := service.NewSettingsService(db)
	sessionMgr := adapterAuth.NewSessionManager(1 * time.Hour)
	authSvc := service.NewAuthService(sessionMgr, settingsSvc, userSvc)

	router := NewRouter(RouterConfig{
		UserSvc: userSvc,
		AuthSvc: authSvc,
	})

	return router, userSvc, authSvc
}

func TestUserHandler_REST(t *testing.T) {
	router, _, authSvc := setupTestUserHTTP(t)

	// 1. Login as admin to get session token
	loginResp, err := authSvc.Login(context.Background(), domain.LoginRequest{
		Username: "admin",
		Password: "admin",
		Source:   domain.AuthSourceLocal,
	})
	if err != nil {
		t.Fatalf("failed login: %v", err)
	}
	token := loginResp.Token

	// 2. GET /api/users should list admin user
	req := httptest.NewRequest("GET", "/api/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var users []*domain.User
	if err := json.Unmarshal(rec.Body.Bytes(), &users); err != nil {
		t.Fatalf("failed unmarshaling users: %v", err)
	}
	if len(users) != 1 || users[0].Username != "admin" {
		t.Fatalf("unexpected users list: %+v", users)
	}

	// 3. POST /api/users to create a new operator
	createPayload := domain.CreateUserRequest{
		Username:    "operator_john",
		DisplayName: "John Operator",
		Email:       "john@redwolf.internal",
		Password:    "SecurePass2026!",
		Role:        domain.RoleOperator,
	}
	body, _ := json.Marshal(createPayload)
	req = httptest.NewRequest("POST", "/api/users", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}

	var created domain.User
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed unmarshaling created user: %v", err)
	}
	if created.Username != "operator_john" || created.Role != domain.RoleOperator {
		t.Fatalf("unexpected created user: %+v", created)
	}

	// 4. GET /api/users/{id}
	req = httptest.NewRequest("GET", "/api/users/"+created.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	// 5. PUT /api/users/{id} to update metadata
	updatePayload := domain.UpdateUserRequest{
		DisplayName: "Johnathan Operator Senior",
		Email:       "john.senior@redwolf.internal",
		Role:        domain.RoleOperator,
	}
	body, _ = json.Marshal(updatePayload)
	req = httptest.NewRequest("PUT", "/api/users/"+created.ID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on update, got %d: %s", rec.Code, rec.Body.String())
	}

	// 6. POST /api/users/{id}/password to change password
	passPayload := domain.ChangePasswordRequest{NewPassword: "BrandNewPassword2026!"}
	body, _ = json.Marshal(passPayload)
	req = httptest.NewRequest("POST", "/api/users/"+created.ID+"/password", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on password change, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify login works with new password
	opLogin, err := authSvc.Login(context.Background(), domain.LoginRequest{
		Username: "operator_john",
		Password: "BrandNewPassword2026!",
		Source:   domain.AuthSourceLocal,
	})
	if err != nil {
		t.Fatalf("expected successful login with new password: %v", err)
	}
	if opLogin.User.Username != "operator_john" {
		t.Fatalf("unexpected login user: %+v", opLogin.User)
	}

	// 7. DELETE /api/users/{id} (cannot delete last admin)
	req = httptest.NewRequest("DELETE", "/api/users/"+loginResp.User.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden deleting self/last admin, got %d: %s", rec.Code, rec.Body.String())
	}

	// Delete created operator
	req = httptest.NewRequest("DELETE", "/api/users/"+created.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK deleting operator, got %d: %s", rec.Code, rec.Body.String())
	}
}
