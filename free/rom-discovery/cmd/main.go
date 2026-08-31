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
	"github.com/nself-org/nself-rom-discovery/internal"
)

func main() {
	cfg := internal.LoadConfig()

	ctx := context.Background()
	db, err := internal.NewDB(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.InitSchema(ctx); err != nil {
		log.Fatalf("failed to initialize schema: %v", err)
	}

	h := internal.NewHandlers(db)
	r := newRouter(h)
	srv := newServer(cfg, r)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("rom-discovery listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-quit
	if err := shutdown(srv, 10*time.Second); err != nil {
		log.Fatalf("graceful shutdown failed: %v", err)
	}
}

// shutdown gracefully stops srv, bounded by timeout. Extracted from main()
// so the shutdown-context wiring and error propagation are directly
// testable via a stub server (see shutdowner in main_test.go).
func shutdown(srv shutdowner, timeout time.Duration) error {
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return err
	}
	log.Println("server stopped")
	return nil
}

// shutdowner is the subset of *http.Server used by shutdown(), extracted so
// tests can substitute a stub instead of binding a real listener.
type shutdowner interface {
	Shutdown(ctx context.Context) error
}

// newRouter wires up all HTTP routes and middleware for the rom-discovery
// plugin. Extracted from main() so it can be exercised directly in tests
// (route registration + middleware wiring) without booting a real server or
// connecting to a database.
func newRouter(h *internal.Handlers) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Get("/health", h.HealthCheck)

	r.Route("/api/v1", func(r chi.Router) {
		// ROM Metadata
		r.Get("/metadata", h.ListMetadata)
		r.Post("/metadata", h.CreateMetadata)
		r.Get("/metadata/{id}", h.GetMetadata)
		r.Delete("/metadata/{id}", h.DeleteMetadata)

		// Download Queue
		r.Get("/downloads", h.ListDownloadQueue)
		r.Post("/downloads", h.CreateDownloadQueue)
		r.Patch("/downloads/{id}", h.UpdateDownloadQueue)

		// Scraper Jobs
		r.Get("/scrapers", h.ListScraperJobs)
		r.Post("/scrapers", h.CreateScraperJob)
		r.Patch("/scrapers/{id}", h.UpdateScraperJob)

		// Popularity
		r.Get("/popularity", h.ListPopularity)
		r.Post("/popularity", h.UpsertPopularity)

		// Audit Log
		r.Get("/audit", h.ListAuditLog)
		r.Post("/audit", h.CreateAuditLog)

		// Legal Acceptance
		r.Get("/legal", h.ListLegalAcceptance)
		r.Post("/legal", h.CreateLegalAcceptance)
	})

	return r
}

// newServer builds the *http.Server for the rom-discovery plugin. Extracted
// from main() so the address + timeout wiring is directly testable without
// booting a real listener.
func newServer(cfg internal.Config, h http.Handler) *http.Server {
	return &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      h,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}
