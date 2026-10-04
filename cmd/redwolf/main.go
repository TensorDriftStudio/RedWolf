package main

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	adapterAuth "github.com/tensordriftstudio/redwolf/internal/adapter/auth"
	adapterDNS "github.com/tensordriftstudio/redwolf/internal/adapter/dnsmasq"
	adapterEvent "github.com/tensordriftstudio/redwolf/internal/adapter/event"
	adapterHTTP "github.com/tensordriftstudio/redwolf/internal/adapter/http"
	adapterSQLite "github.com/tensordriftstudio/redwolf/internal/adapter/sqlite"
	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/service"
)

func main() {
	// Initialize structured JSON logging (Enterprise standard)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("initializing RedWolf Bare-Metal Core Appliance")

	// Read environment variables
	httpPortStr := getEnv("REDWOLF_HTTP_PORT", "8080")
	httpPort, err := strconv.Atoi(httpPortStr)
	if err != nil {
		slog.Error("invalid REDWOLF_HTTP_PORT", "val", httpPortStr, "error", err)
		os.Exit(1)
	}

	dbPath := getEnv("REDWOLF_DB_PATH", "")
	if dbPath == "" {
		if _, err := os.Stat("/var/lib/redwolf/db"); err == nil {
			dbPath = "/var/lib/redwolf/db/redwolf.db"
		} else {
			dbPath = "data/redwolf.db"
		}
	}

	imageDir := getEnv("REDWOLF_IMAGE_DIR", "")
	if imageDir == "" {
		if _, err := os.Stat("/var/lib/redwolf/images"); err == nil {
			imageDir = "/var/lib/redwolf/images"
		} else {
			imageDir = "data/images"
		}
	}
	provIface := getEnv("REDWOLF_PROVISIONING_INTERFACE", "eth0")
	serverURL := getEnv("REDWOLF_SERVER_URL", fmt.Sprintf("http://127.0.0.1:%d", httpPort))

	// Ensure parent data directories exist
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		slog.Error("failed to create db directory", "path", filepath.Dir(dbPath), "error", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		slog.Error("failed to create image directory", "path", imageDir, "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Initialize SQLite Database (WAL mode)
	repo, err := adapterSQLite.NewRepository(dbPath)
	if err != nil {
		slog.Error("failed to initialize sqlite repository", "path", dbPath, "error", err)
		os.Exit(1)
	}
	defer repo.Close()
	slog.Info("sqlite database initialized in WAL mode", "path", dbPath)

	// 2. Initialize Event Bus
	eventBus := adapterEvent.NewBroadcaster()

	// 3. Initialize Settings Service & Directory Provider
	settingsSvc := service.NewSettingsService(repo.DB())

	// 4. Initialize User Repository, User Service & Default Admin Bootstrap
	userRepo := adapterSQLite.NewUserRepository(repo.DB())
	userSvc := service.NewUserService(userRepo)
	if err := userSvc.EnsureDefaultAdmin(ctx); err != nil {
		slog.Error("failed bootstrapping default administrator", "error", err)
	}

	// 5. Initialize Session Manager & Multi-Provider Auth Service
	sessionMgr := adapterAuth.NewSessionManager(24 * time.Hour)
	authSvc := service.NewAuthService(sessionMgr, settingsSvc, userSvc)

	// 6. Initialize Core Domain Provisioner, BMC Escrow Vault & BMC Power Manager
	prov := service.NewProvisioner(repo, eventBus)
	vaultKey := getEnv("REDWOLF_VAULT_KEY", "redwolf-master-key-datacenter-default-256")
	if os.Getenv("REDWOLF_VAULT_KEY") == "" {
		slog.Warn("REDWOLF_VAULT_KEY not set; using default datacenter encryption key. Configure a custom master key in production environments.")
	}
	bmcEscrow := service.NewBMCEscrowService(repo.DB(), vaultKey, repo, eventBus)
	bmcMgr := service.NewBMCManager(repo, bmcEscrow)
	bmcEscrow.SetBMCManager(bmcMgr)

	// 7. Initialize Cloud-Init Template Service & OS Image Catalog
	templateSvc := service.NewTemplateService(repo.DB())
	prov.SetTemplateService(templateSvc)
	imageCatalog := service.NewImageCatalogService(imageDir)
	prov.SetImageCatalog(imageCatalog)

	// 8. Initialize & Start Managed dnsmasq Supervisor with persisted network settings
	initSettings, _ := settingsSvc.GetSettings(ctx)
	if initSettings != nil && initSettings.General.ServerURL != "" && (serverURL == "" || strings.Contains(serverURL, "127.0.0.1") || strings.Contains(serverURL, "localhost")) {
		serverURL = initSettings.General.ServerURL
		slog.Info("using server URL from persisted settings", "url", serverURL)
	}
	var netCfg domain.NetworkSettings
	if initSettings != nil {
		netCfg = initSettings.Network
	}

	dnsmasqMgr := adapterDNS.NewManager(adapterDNS.Config{
		Interface:            provIface,
		HTTPPort:             httpPort,
		TFTPDir:              getEnv("REDWOLF_TFTP_DIR", "/var/lib/redwolf/tftp"),
		ConfDir:              getEnv("REDWOLF_CONF_DIR", "/etc/redwolf"),
		LogDir:               getEnv("REDWOLF_LOG_DIR", "/var/log/redwolf"),
		SubnetCIDR:           netCfg.SubnetCIDR,
		DHCPRangeStart:       netCfg.DHCPRangeStart,
		DHCPRangeEnd:         netCfg.DHCPRangeEnd,
		Gateway:              netCfg.Gateway,
		DNSServers:           netCfg.DNSServers,
		LeaseDurationMinutes: netCfg.LeaseDurationMinutes,
	})

	if err := dnsmasqMgr.Start(ctx); err != nil {
		slog.Warn("could not start dnsmasq manager; continuing in standalone API mode", "error", err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer stopCancel()
		_ = dnsmasqMgr.Stop(stopCtx)
	}()

	// 9. Detect Web Dashboard static assets (/usr/share/redwolf/web in container, web/dist locally)
	var webFS fs.FS
	for _, candidate := range []string{"/usr/share/redwolf/web", "web/dist", "dist"} {
		if _, err := os.Stat(filepath.Join(candidate, "index.html")); err == nil {
			webFS = os.DirFS(candidate)
			slog.Info("mounted static web dashboard", "path", candidate)
			break
		}
	}

	// 10. Initialize Router & HTTP Server
	router := adapterHTTP.NewRouter(adapterHTTP.RouterConfig{
		Provisioner:  prov,
		AuthSvc:      authSvc,
		UserSvc:      userSvc,
		SettingsSvc:  settingsSvc,
		BMCEscrow:    bmcEscrow,
		BMCManager:   bmcMgr,
		Templates:    templateSvc,
		ImageCatalog: imageCatalog,
		Events:       eventBus,
		DNSMasq:      dnsmasqMgr,
		ServerURL:    serverURL,
		ImageDir:     imageDir,
		WebFS:        webFS,
	})

	serverAddr := fmt.Sprintf(":%d", httpPort)
	srv := adapterHTTP.NewServer(serverAddr, router)


	go func() {
		if err := srv.Start(); err != nil {
			slog.Error("http server stopped unexpectedly", "error", err)
			cancel()
		}
	}()

	slog.Info("RedWolf Core Appliance ready", "port", httpPort, "interface", provIface)

	// Wait for OS shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		slog.Info("received termination signal", "signal", sig.String())
	case <-ctx.Done():
		slog.Info("server context cancelled")
	}

	// Graceful shutdown sequence
	slog.Info("initiating graceful shutdown")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("error during http server shutdown", "error", err)
	}

	slog.Info("RedWolf Core shutdown complete")
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
