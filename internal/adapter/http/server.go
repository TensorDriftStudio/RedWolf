package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Server encapsulates the HTTP server runtime.
type Server struct {
	httpServer *http.Server
}

// NewServer initializes an HTTP server with production timeout guards.
func NewServer(addr string, handler http.Handler) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       60 * time.Second,
			WriteTimeout:      0, // Disabled for high-throughput OS image streaming and WebSocket events
			IdleTimeout:       120 * time.Second,
		},
	}
}

// Start listens and serves incoming HTTP requests until context cancellation or fatal error.
func (s *Server) Start() error {
	slog.Info("starting HTTP server", "addr", s.httpServer.Addr)
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server failure: %w", err)
	}
	return nil
}

// Shutdown gracefully finishes in-flight requests within the provided context timeout.
func (s *Server) Shutdown(ctx context.Context) error {
	slog.InfoContext(ctx, "shutting down HTTP server")
	return s.httpServer.Shutdown(ctx)
}
