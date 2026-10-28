// Package server wires up the HTTP server.
package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/nself-org/nself-siem/internal/handlers"
)

// Server wraps net/http.Server.
type Server struct {
	inner *http.Server
}

// New creates and configures the HTTP server.
// migrationsHealth, when not nil, supplies the "migrations" member of /health (it may write a 503 header).
func New(host, port string, h *handlers.Handler, migrationsHealth func(http.ResponseWriter) map[string]int) *Server {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := map[string]any{"status": "ok"}
		if migrationsHealth != nil {
			if m := migrationsHealth(w); m != nil {
				body["migrations"] = m
			}
		}
		_ = json.NewEncoder(w).Encode(body)
	})

	h.RegisterRoutes(r)

	return &Server{
		inner: &http.Server{
			Addr:         net.JoinHostPort(host, port),
			Handler:      r,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
	}
}

// Start listens and serves; returns when ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		slog.Info("SIEM agent listening", "addr", s.inner.Addr)
		errCh <- s.inner.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.inner.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}
