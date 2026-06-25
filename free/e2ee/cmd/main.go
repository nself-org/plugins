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

// Size-cap exception: plugin entry-point main() — 78L startup wiring (env/db/router/server); single invocation, not a reusable unit.
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

	// All key-directory routes require the gateway-forwarded authenticated
	// principal. internal.RequireAuth extracts X-Hasura-User-Id + the source
	// account and FAILS CLOSED (401) on any request missing an authenticated
	// user id. Hasura/nginx is the trust boundary; port 3055 must NOT be
	// publicly routed (gateway-only ingress).
	r.Group(func(pr chi.Router) {
		pr.Use(internal.RequireAuth)

		// Key directory
		pr.Post("/api/v1/e2ee/identity/register", h.RegisterIdentity)
		pr.Post("/api/v1/e2ee/signed-prekey", h.UploadSignedPreKey)
		pr.Post("/api/v1/e2ee/one-time-prekeys", h.UploadOneTimePreKeys)
		pr.Get("/api/v1/e2ee/bundle/{userId}", h.GetPreKeyBundle)
		pr.Get("/api/v1/e2ee/replenish/{userId}", h.CheckReplenish)

		// Verification / safety numbers
		pr.Post("/api/v1/e2ee/safety-number", h.PostSafetyNumber)
		pr.Get("/api/v1/e2ee/verification/{userId}/{peerId}", h.GetVerificationState)

		// Audit
		pr.Get("/api/v1/e2ee/audit/{userId}", h.ListAudit)
	})

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
