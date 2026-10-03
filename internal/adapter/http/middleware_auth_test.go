package http

import (
	"context"
	"database/sql"
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

func TestAuthMiddleware_Enforcement(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed opening in-memory db: %v", err)
	}
	defer db.Close()

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
		UserSvc:     userSvc,
		AuthSvc:     authSvc,
		SettingsSvc: settingsSvc,
	})

	// 1. Unauthenticated request to /api/users should return 401 Unauthorized
	req := httptest.NewRequest("GET", "/api/users", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for unauthenticated /api/users, got %d", rec.Code)
	}

	// 2. Unauthenticated request to /api/settings should return 401 Unauthorized
	reqSettings := httptest.NewRequest("GET", "/api/settings", nil)
	recSettings := httptest.NewRecorder()
	router.ServeHTTP(recSettings, reqSettings)

	if recSettings.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for unauthenticated /api/settings, got %d", recSettings.Code)
	}

	// 3. Login as admin
	loginResp, err := authSvc.Login(context.Background(), domain.LoginRequest{
		Username: "admin",
		Password: "admin",
		Source:   domain.AuthSourceLocal,
	})
	if err != nil {
		t.Fatalf("failed admin login: %v", err)
	}

	// 4. Authenticated admin request to /api/settings should return 200 OK
	reqAuthed := httptest.NewRequest("GET", "/api/settings", nil)
	reqAuthed.Header.Set("Authorization", "Bearer "+loginResp.Token)
	recAuthed := httptest.NewRecorder()
	router.ServeHTTP(recAuthed, reqAuthed)

	if recAuthed.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for authenticated admin /api/settings, got %d: %s", recAuthed.Code, recAuthed.Body.String())
	}
}
