package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/nself-org/nself-devices/internal"
)

// newRouter builds the chi router with all devices routes wired to h.
// Extracted from main() so route wiring is unit-testable via httptest without
// starting a real listener or touching a live database.
func newRouter(h *internal.Handlers) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	// Health
	r.Get("/health", h.Health)
	r.Get("/ready", h.Ready)

	// v1 API
	r.Route("/api/v1", func(r chi.Router) {
		// Devices
		r.Get("/devices", h.ListDevices)
		r.Post("/devices", h.CreateDevice)
		r.Get("/devices/{id}", h.GetDevice)
		r.Put("/devices/{id}", h.UpdateDevice)
		r.Delete("/devices/{id}", h.DeleteDevice)

		// Device commands
		r.Post("/devices/{id}/commands", h.SendDeviceCommand)
		r.Get("/devices/{id}/commands", h.ListDeviceCommands)

		// Device telemetry
		r.Get("/devices/{id}/telemetry", h.GetDeviceTelemetry)

		// Commands
		r.Get("/commands/{id}", h.GetCommand)

		// Telemetry
		r.Post("/telemetry", h.IngestTelemetry)
		r.Post("/telemetry/batch", h.BatchIngestTelemetry)
		r.Get("/telemetry", h.QueryTelemetry)

		// Ingest sessions
		r.Get("/ingest-sessions", h.ListIngestSessions)
		r.Post("/ingest-sessions", h.CreateIngestSession)

		// Audit
		r.Get("/audit", h.GetAuditLog)

		// Stats
		r.Get("/stats", h.GetStats)
	})

	return r
}

// serverAddr formats the listen address from config. Extracted so the
// host:port formatting rule is unit-testable in isolation.
func serverAddr(cfg internal.Config) string {
	return fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
}

// newServer builds the http.Server for addr wrapping r. Extracted for
// unit-testability of the timeout/address wiring without binding a socket.
func newServer(addr string, r *chi.Mux) *http.Server {
	return &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// requireDatabaseURL validates that a DATABASE_URL was configured, returning
// an error instead of calling log.Fatal so the validation rule is
// unit-testable without terminating the test process.
func requireDatabaseURL(cfg internal.Config) error {
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL environment variable is required")
	}
	return nil
}

// shutdownServer gracefully shuts down srv with a bounded timeout. Extracted
// so the graceful-shutdown branch is unit-testable against an httptest server
// without relying on OS signals.
func shutdownServer(srv *http.Server, timeout time.Duration) error {
	shutCtx, shutCancel := context.WithTimeout(context.Background(), timeout)
	defer shutCancel()
	return srv.Shutdown(shutCtx)
}

func main() {
	cfg := internal.LoadConfig()

	if err := requireDatabaseURL(cfg); err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	db, err := internal.NewDB(ctx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		log.Fatalf("database connect: %v", err)
	}
	defer db.Close()

	if err := db.InitSchema(context.Background()); err != nil {
		log.Fatalf("schema init: %v", err)
	}

	h := &internal.Handlers{DB: db, Cfg: cfg}

	r := newRouter(h)

	addr := serverAddr(cfg)
	srv := newServer(addr, r)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("devices plugin listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down...")
	if err := shutdownServer(srv, 5*time.Second); err != nil {
		log.Printf("graceful shutdown error: %v", err)
	}
	log.Println("devices plugin stopped")
}
