package http

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/tensordriftstudio/redwolf/internal/domain"
	"github.com/tensordriftstudio/redwolf/internal/port"
	"github.com/tensordriftstudio/redwolf/internal/service"
	"github.com/tensordriftstudio/redwolf/internal/version"
)

// RouterConfig holds settings for route initialization.
type RouterConfig struct {
	Provisioner  *service.Provisioner
	AuthSvc      *service.AuthService
	UserSvc      *service.UserService
	SettingsSvc  *service.SettingsService
	BMCEscrow    *service.BMCEscrowService
	BMCManager   *service.BMCManager
	Templates    *service.TemplateService
	ImageCatalog *service.ImageCatalogService
	Events       port.EventBroadcaster
	DNSMasq      port.DNSMasqManager
	ServerURL    string
	ImageDir     string
	WebFS        fs.FS // Optional embedded frontend filesystem
}

// NewRouter builds and wires the Chi HTTP multiplexer.
func NewRouter(cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	// Base middlewares
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Permissive CORS for development dashboard access
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Healthcheck for Docker/Kubernetes probes
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// RedWolf Enterprise Version metadata endpoint
	r.Get("/api/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, version.Get())
	})

	// Dynamic iPXE bootloader script endpoint
	ipxeHandler := NewIPXEHandler(cfg.Provisioner, cfg.ServerURL)
	r.Get("/boot.ipxe", ipxeHandler.ServeHTTP)

	// Real-time WebSocket event bus
	wsHandler := NewWSHandler(cfg.Events)
	r.Get("/ws/events", wsHandler.ServeHTTP)

	// Node Management REST API
	nodeHandler := NewNodeHandler(cfg.Provisioner)
	deployHandler := NewDeployHandler(cfg.Provisioner)

	// Authentication REST API
	if cfg.AuthSvc != nil {
		authHandler := NewAuthHandler(cfg.AuthSvc)
		r.Route("/api/auth", func(r chi.Router) {
			r.Post("/login", authHandler.Login)
			r.Get("/me", authHandler.Me)
			r.Post("/logout", authHandler.Logout)
		})
	}

	// User Management REST API (Requires Admin role)
	if cfg.UserSvc != nil {
		userHandler := NewUserHandler(cfg.UserSvc, cfg.AuthSvc)
		r.Route("/api/users", func(r chi.Router) {
			if cfg.AuthSvc != nil {
				r.Use(Authenticator(cfg.AuthSvc))
				r.Use(RequireRole(domain.RoleAdmin))
			}
			r.Get("/", userHandler.List)
			r.Post("/", userHandler.Create)
			r.Get("/{id}", userHandler.Get)
			r.Put("/{id}", userHandler.Update)
			r.Post("/{id}/password", userHandler.ChangePassword)
			r.Delete("/{id}", userHandler.Delete)
		})
	}

	// Settings & Directory Testing REST API (Requires Admin role)
	if cfg.SettingsSvc != nil {
		settingsHandler := NewSettingsHandler(cfg.SettingsSvc, cfg.DNSMasq)
		r.Route("/api/settings", func(r chi.Router) {
			if cfg.AuthSvc != nil {
				r.Use(Authenticator(cfg.AuthSvc))
				r.Use(RequireRole(domain.RoleAdmin))
			}
			r.Get("/", settingsHandler.Get)
			r.Put("/", settingsHandler.Update)
			r.Post("/test-directory", settingsHandler.TestDirectory)
		})
	}

	// Cloud-Init Templates REST API (Requires Authentication)
	if cfg.Templates != nil {
		templateHandler := NewTemplateHandler(cfg.Templates)
		r.Route("/api/templates", func(r chi.Router) {
			if cfg.AuthSvc != nil {
				r.Use(Authenticator(cfg.AuthSvc))
			}
			r.Get("/", templateHandler.List)
			r.Post("/", templateHandler.Create)
			r.Get("/{id}", templateHandler.Get)
			r.Put("/{id}", templateHandler.Update)
			r.Delete("/{id}", templateHandler.Delete)
		})
	}

	// OS Image Catalog REST API (Requires Authentication)
	if cfg.ImageCatalog != nil {
		imageHandler := NewImageHandler(cfg.ImageCatalog)
		r.Route("/api/images", func(r chi.Router) {
			if cfg.AuthSvc != nil {
				r.Use(Authenticator(cfg.AuthSvc))
			}
			r.Get("/", imageHandler.List)
			r.Post("/download", imageHandler.Download)
			r.Get("/status", imageHandler.Status)
		})
	}

	var bmcHandler *BMCHandler
	if cfg.BMCEscrow != nil || cfg.BMCManager != nil {
		bmcHandler = NewBMCHandler(cfg.BMCEscrow, cfg.BMCManager)
	}

	r.Route("/api/nodes", func(r chi.Router) {
		// Agent endpoints (unauthenticated by design for in-memory PXE discovery agent)
		r.Post("/telemetry", nodeHandler.IngestTelemetry)
		r.Get("/task/by-mac/{mac}", deployHandler.GetTaskByMAC)
		r.Post("/{id}/progress", deployHandler.UpdateProgress)

		// Operator & Admin Fleet Management endpoints (Authenticated)
		r.Group(func(r chi.Router) {
			if cfg.AuthSvc != nil {
				r.Use(Authenticator(cfg.AuthSvc))
			}
			r.Get("/", nodeHandler.List)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", nodeHandler.Get)
				r.Delete("/", nodeHandler.Delete)
				r.Post("/reset", nodeHandler.Reset)
				r.Post("/deploy", deployHandler.Deploy)
				r.Get("/task", deployHandler.GetTask)
				if bmcHandler != nil {
					r.Post("/bmc/rotate", bmcHandler.Rotate)
					r.Get("/bmc/credentials", bmcHandler.GetCredentials)
					r.Get("/power", bmcHandler.GetPower)
					r.Post("/power", bmcHandler.SetPower)
				}
			})
		})
	})

	// Static high-speed OS image and discovery kernel file server (ImageDir with comprehensive fallback)
	assetHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		relPath := strings.TrimPrefix(r.URL.Path, "/assets/")
		candidates := []string{}
		if cfg.ImageDir != "" {
			candidates = append(candidates, cfg.ImageDir)
		}
		candidates = append(candidates, "assets", "/usr/share/redwolf/assets", "data/images", "/var/lib/redwolf/images")

		for _, dir := range candidates {
			// Direct path resolution
			target := filepath.Join(dir, relPath)
			if info, err := os.Stat(target); err == nil && !info.IsDir() {
				http.ServeFile(w, r, target)
				return
			}

			// If relPath starts with "images/" and dir already represents an images directory, resolve subpath
			if strings.HasPrefix(relPath, "images/") {
				subPath := strings.TrimPrefix(relPath, "images/")
				targetSub := filepath.Join(dir, subPath)
				if info, err := os.Stat(targetSub); err == nil && !info.IsDir() {
					http.ServeFile(w, r, targetSub)
					return
				}
			}
		}

		// Check WebFS for compiled dashboard assets (e.g. Vite /assets/index-*.js)
		if cfg.WebFS != nil {
			if _, err := fs.Stat(cfg.WebFS, filepath.Join("assets", relPath)); err == nil {
				http.FileServer(http.FS(cfg.WebFS)).ServeHTTP(w, r)
				return
			}
		}

		http.NotFound(w, r)
	})
	r.Handle("/assets/*", assetHandler)

	// Embedded or local Web Dashboard static file server
	if cfg.WebFS != nil {
		serveEmbeddedUI(r, cfg.WebFS)
	}

	return r
}

func serveEmbeddedUI(r *chi.Mux, webFS fs.FS) {
	fileServer := http.FileServer(http.FS(webFS))

	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		path := strings.TrimPrefix(req.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// If file exists in embedded filesystem, serve it directly
		if _, err := fs.Stat(webFS, path); err == nil {
			fileServer.ServeHTTP(w, req)
			return
		}

		// Fallback to index.html for Single Page Application client-side routing
		indexData, err := fs.ReadFile(webFS, "index.html")
		if err != nil {
			http.NotFound(w, req)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexData)
	})
}
