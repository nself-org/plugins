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
	"github.com/nself-org/nself-compliance/internal"
)

// newRouter builds the chi router with all middleware and routes wired up.
// Extracted from main() so it can be exercised via httptest without binding
// a real port or connecting to a database (db.Pool may be a mock for
// route-shape tests; individual handler behavior is covered in the internal
// package). Mirrors the pattern established in paid/moderation and paid/web3.
func newRouter(db *internal.DB) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// Health
	r.Get("/health", internal.HealthHandler)
	r.Get("/ready", internal.ReadyHandler(db))

	// API v1
	r.Route("/api/v1", func(r chi.Router) {
		// DSARs
		r.Get("/dsars", internal.ListDSARsHandler(db))
		r.Post("/dsars", internal.CreateDSARHandler(db))
		r.Get("/dsars/{id}", internal.GetDSARHandler(db))
		r.Put("/dsars/{id}", internal.UpdateDSARHandler(db))
		r.Post("/dsars/{id}/complete", internal.CompleteDSARHandler(db))
		r.Post("/dsars/{id}/deny", internal.DenyDSARHandler(db))
		r.Get("/dsars/{id}/activities", internal.ListDSARActivitiesHandler(db))

		// Consents
		r.Get("/consents", internal.ListConsentsHandler(db))
		r.Post("/consents", internal.CreateConsentHandler(db))
		r.Get("/consents/{id}", internal.GetConsentHandler(db))
		r.Post("/consents/revoke", internal.RevokeConsentHandler(db))

		// User consents
		r.Get("/users/{user_id}/consents", internal.ListUserConsentsHandler(db))

		// Privacy Policies
		r.Get("/privacy-policies", internal.ListPrivacyPoliciesHandler(db))
		r.Post("/privacy-policies", internal.CreatePrivacyPolicyHandler(db))
		r.Get("/privacy-policies/{id}", internal.GetPrivacyPolicyHandler(db))

		// Data Exports
		r.Post("/data-exports", internal.CreateDataExportHandler(db))
		r.Get("/data-exports/{id}", internal.GetDataExportHandler(db))

		// Stats
		r.Get("/stats", internal.StatsHandler(db))

		// SOC 2
		r.Get("/compliance/soc2/controls", internal.SOC2ControlsHandler(db))
		r.Post("/compliance/soc2/evidence", internal.CreateEvidencePackHandler(db))

		// Change log (CC7)
		r.Get("/compliance/change-log", internal.ListChangeLogHandler(db))
		r.Post("/compliance/change-log", internal.CreateChangeLogHandler(db))

		// Access reviews (CC6)
		r.Get("/compliance/access-reviews", internal.ListAccessReviewsHandler(db))
		r.Post("/compliance/access-reviews", internal.CreateAccessReviewHandler(db))
		r.Patch("/compliance/access-reviews/{id}", internal.PatchAccessReviewHandler(db))
	})

	return r
}

func main() {
	cfg := internal.Load()

	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := internal.NewDB(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	if err := db.InitSchema(context.Background()); err != nil {
		log.Fatalf("init schema: %v", err)
	}

	if err := db.InitSOC2Schema(context.Background()); err != nil {
		log.Fatalf("init soc2 schema: %v", err)
	}

	r := newRouter(db)

	addr := fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("compliance plugin listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down compliance plugin...")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	log.Println("compliance plugin stopped")
}
