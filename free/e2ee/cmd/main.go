// Command nself-e2ee runs the E2EE key-directory HTTP service.
//
// Purpose:   Boot the chi router for the X3DH + Kyber-1024 prekey directory.
// Inputs:    DATABASE_URL (required), E2EE_PLUGIN_PORT/HOST (optional).
// Outputs:   HTTP server on cfg.Port; graceful shutdown on SIGINT/SIGTERM.
// Constraints: This process stores PUBLIC keys only. Request bodies for key
//   endpoints are NOT logged (no body-logging middleware is installed) so no
//   key material can leak into logs.
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

	"github.com/nself-org/nself-e2ee/internal"
)

func main() {
	cfg := internal.LoadConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := internal.NewPool(ctx, cfg)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer pool.Close()

	h := internal.NewHandlersFromConfig(pool, cfg)

	r := chi.NewRouter()
	// NOTE: middleware.Logger logs method + path + status only — it never logs
	// request bodies, so PUBLIC key payloads are not written to logs. Do NOT add
	// a body-logging middleware to this router.
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Health
	r.Get("/health", h.Health)
	r.Get("/ready", h.Ready)

	// Key directory
	r.Post("/api/v1/e2ee/identity/register", h.RegisterIdentity)
	r.Post("/api/v1/e2ee/signed-prekey", h.UploadSignedPreKey)
	r.Post("/api/v1/e2ee/one-time-prekeys", h.UploadOneTimePreKeys)
	r.Get("/api/v1/e2ee/bundle/{userId}", h.GetPreKeyBundle)
	r.Get("/api/v1/e2ee/replenish/{userId}", h.CheckReplenish)

	// Verification / safety numbers
	r.Post("/api/v1/e2ee/safety-number", h.PostSafetyNumber)
	r.Get("/api/v1/e2ee/verification/{userId}/{peerId}", h.GetVerificationState)

	// Audit
	r.Get("/api/v1/e2ee/audit/{userId}", h.ListAudit)

	addr := ":" + cfg.Port
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("nself-e2ee listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down...")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		log.Fatalf("graceful shutdown failed: %v", err)
	}
	log.Println("server stopped")
}
