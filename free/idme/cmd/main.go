package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/nself-org/nself-idme/internal"
)

// newRouter builds the chi router with all middleware and routes wired up.
// Extracted from main() so it can be exercised via httptest without binding
// a real port or connecting to a database (db.Pool may be a pgxmock pool for
// route-shape tests; individual handler behavior is covered in the internal
// package).
func newRouter(db *internal.DB, cfg internal.Config) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	// Health
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","service":"idme-plugin"}`))
	})

	// API v1
	r.Route("/api/v1", func(r chi.Router) {
		// Verifications
		r.Post("/verifications", internal.CreateVerification(db))
		r.Get("/verifications", internal.ListVerifications(db))
		r.Get("/verifications/{id}", internal.GetVerification(db))
		r.Post("/verifications/callback", internal.VerificationCallback(db, cfg))

		// Groups
		r.Get("/groups", internal.ListGroups(db))
		r.Post("/groups", internal.CreateGroup(db))
		r.Put("/groups/{id}", internal.UpdateGroup(db))
		r.Delete("/groups/{id}", internal.DeleteGroup(db))

		// User sub-resources
		r.Get("/users/{user_id}/verifications", internal.GetUserVerifications(db))
		r.Get("/users/{user_id}/badges", internal.GetUserBadges(db))

		// Badges
		r.Get("/badges", internal.ListBadges(db))
		r.Post("/badges", internal.CreateBadge(db))

		// Stats
		r.Get("/stats", internal.GetStats(db))
	})

	return r
}

// newServer builds the *http.Server with production timeouts, given a
// pre-built handler. Extracted from main() so the server configuration
// (address, timeouts) is independently testable without binding a port.
func newServer(handler http.Handler, cfg internal.Config) *http.Server {
	return &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// runUntilSignal starts srv in the background and blocks until a signal is
// received on quit, then gracefully shuts srv down. Extracted from main() so
// the start/wait/shutdown sequence is testable against a real ephemeral-port
// listener without depending on OS signal delivery in main().
func runUntilSignal(srv *http.Server, quit <-chan os.Signal) error {
	go func() {
		log.Printf("idme-plugin listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("server error: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down idme-plugin...")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		return err
	}
	log.Println("idme-plugin stopped")
	return nil
}

func main() {
	cfg := internal.LoadConfig()

	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := internal.NewDB(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	r := newRouter(db, cfg)
	srv := newServer(r, cfg)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	if err := runUntilSignal(srv, quit); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}
}
