package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/tensordriftstudio/redwolf/internal/adapter/auth"
	"github.com/tensordriftstudio/redwolf/internal/domain"
)

// SettingsService manages system settings and directory testing.
type SettingsService struct {
	db       *sql.DB
	mu       sync.RWMutex
	cached   *domain.SystemSettings
}

// NewSettingsService creates an initialized settings service.
func NewSettingsService(db *sql.DB) *SettingsService {
	svc := &SettingsService{
		db: db,
	}
	svc.cached = svc.defaultSettings()
	svc.initSchema()
	_ = svc.loadFromDB(context.Background())
	return svc
}

func (s *SettingsService) defaultSettings() *domain.SystemSettings {
	return &domain.SystemSettings{
		General: domain.GeneralSettings{
			ApplianceName:         "RedWolf Bare-Metal Appliance",
			ServerURL:             "http://192.168.0.250:8080",
			ProvisioningInterface: "eth0",
			DefaultOS:             domain.OSAlmaLinux9,
		},
		Network: domain.NetworkSettings{
			SubnetCIDR:           "192.168.0.0/24",
			DHCPRangeStart:       "192.168.0.100",
			DHCPRangeEnd:         "192.168.0.200",
			Gateway:              "192.168.0.1",
			DNSServers:           []string{"1.1.1.1", "8.8.8.8"},
			LeaseDurationMinutes: 1440,
		},
		Auth: domain.AuthSettings{
			LocalAuthEnabled: true,
			LDAP: domain.LDAPConfig{
				Enabled:            false,
				Host:               "ldap.corp.example.com",
				Port:               389,
				UseTLS:             false,
				StartTLS:           true,
				InsecureSkipVerify: false,
				BindDN:             "cn=readonly,dc=corp,dc=example,dc=com",
				BindPassword:       "",
				BaseDN:             "dc=corp,dc=example,dc=com",
				UserFilter:         "(&(objectClass=posixAccount)(uid=%s))",
				GroupSearchDN:      "ou=Groups,dc=corp,dc=example,dc=com",
				AdminGroupDN:       "cn=RedWolf-Admins,ou=Groups,dc=corp,dc=example,dc=com",
				OperatorGroupDN:    "cn=RedWolf-Operators,ou=Groups,dc=corp,dc=example,dc=com",
			},
			ActiveDirectory: domain.ActiveDirectoryConfig{
				Enabled:            false,
				Domain:             "corp.redwolf.internal",
				DomainController:   "dc01.corp.redwolf.internal",
				Port:               636,
				UseLDAPS:           true,
				InsecureSkipVerify: false,
				BindDN:             "svc-redwolf@corp.redwolf.internal",
				BindPassword:       "",
				BaseDN:             "DC=corp,DC=redwolf,DC=internal",
				UserSearchFilter:   "(&(objectClass=user)(|(sAMAccountName=%s)(userPrincipalName=%s)))",
				AdminGroup:         "CN=Domain Admins,CN=Users,DC=corp,DC=redwolf,DC=internal",
				OperatorGroup:      "CN=Server Operators,CN=Builtin,DC=corp,DC=redwolf,DC=internal",
			},
		},
		Storage: domain.StorageSettings{
			ImageStorageDir: "/var/lib/redwolf/images",
			MaxCacheSizeGB:  100,
		},
		UpdatedAt: time.Now().UTC(),
	}
}

func (s *SettingsService) initSchema() {
	if s.db == nil {
		return
	}
	query := `CREATE TABLE IF NOT EXISTS system_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);`
	_, _ = s.db.Exec(query)
}

func (s *SettingsService) loadFromDB(ctx context.Context) error {
	if s.db == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var val string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM system_settings WHERE key = 'current'").Scan(&val)
	if err != nil {
		return err
	}

	var parsed domain.SystemSettings
	if err := json.Unmarshal([]byte(val), &parsed); err != nil {
		return err
	}
	s.cached = &parsed
	return nil
}

// GetSettings retrieves current system settings.
func (s *SettingsService) GetSettings(ctx context.Context) (*domain.SystemSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cached, nil
}

// UpdateSettings stores and applies new system settings.
func (s *SettingsService) UpdateSettings(ctx context.Context, settings domain.SystemSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	settings.UpdatedAt = time.Now().UTC()
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}

	if s.db != nil {
		query := `INSERT INTO system_settings (key, value, updated_at) 
			VALUES ('current', ?, ?) 
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`
		_, err := s.db.ExecContext(ctx, query, string(data), settings.UpdatedAt)
		if err != nil {
			return fmt.Errorf("failed persisting settings: %w", err)
		}
	}

	s.cached = &settings
	slog.InfoContext(ctx, "system settings updated successfully",
		"ldap_enabled", settings.Auth.LDAP.Enabled,
		"ad_enabled", settings.Auth.ActiveDirectory.Enabled,
	)
	return nil
}

// TestDirectory verifies connectivity and search against an LDAP or AD directory.
func (s *SettingsService) TestDirectory(ctx context.Context, req domain.DirectoryTestRequest) *domain.DirectoryTestResult {
	switch req.Source {
	case domain.AuthSourceLDAP:
		cfg := s.cached.Auth.LDAP
		if req.LDAP != nil {
			cfg = *req.LDAP
		}
		return auth.TestLDAPConnection(ctx, cfg)

	case domain.AuthSourceAD:
		cfg := s.cached.Auth.ActiveDirectory
		if req.ActiveDirectory != nil {
			cfg = *req.ActiveDirectory
		}
		return auth.TestADConnection(ctx, cfg)

	default:
		return &domain.DirectoryTestResult{
			Success:   false,
			Message:   fmt.Sprintf("unsupported test directory source: %s", req.Source),
			TestedAt:  time.Now().UTC(),
		}
	}
}
