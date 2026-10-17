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
	"github.com/nself-org/nself-vault/internal/api"
	"github.com/nself-org/nself-vault/internal/auth"
	"github.com/nself-org/nself-vault/internal/crypto"
	"github.com/nself-org/nself-vault/internal/store"
)

func main() {
	if os.Getenv("VAULT_PLUGIN_ENABLED") == "false" || os.Getenv("VAULT_PLUGIN_ENABLED") == "" {
		log.Println("vault: VAULT_PLUGIN_ENABLED is not set to true — exiting")
		os.Exit(0)
	}

	dbURL := mustEnv("DATABASE_URL")
	jwtSecret := mustEnv("NSELF_JWT_SECRET")

	// Load the KEK ring at startup. Failure here is fatal — there is no
	// safe fallback for a vault with no key material. See docs/kek-rotation.md.
	keyRing, err := crypto.Load()
	if err != nil {
		log.Fatalf("vault: KEK load failed: %v", err)
	}
	log.Printf("vault: KEK ring loaded (versions=%v current=V%d)", keyRing.Versions(), keyRing.Current())

	bind := envOr("VAULT_BIND", "127.0.0.1")
	port := envOr("VAULT_PORT", "3823")
	addr := fmt.Sprintf("%s:%s", bind, port)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := store.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("vault: failed to connect to database: %v", err)
	}
	defer db.Close()

	// Own migrations apply once, right after the pool exists; a failure is fatal.
	mustBootMigrations(context.Background(), db.Pool())

	extractor := auth.NewExtractor(jwtSecret)
	h := api.New(db, extractor)

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(redactEnvelopeMiddleware)

	r.Get("/health", handleHealth) // reports the migration ledger counts; h.Health is the plain liveness handler

	r.Route("/vault/v1", func(r chi.Router) {
		// Device management
		r.Post("/devices", h.PostDevice)
		r.Get("/devices", h.GetDevices)
		r.Delete("/devices/{id}", h.DeleteDevice)

		// Record + envelope management
		r.Post("/records", h.PostRecord)
		r.Get("/records", h.GetRecords)
		r.Get("/records/{id}/envelope", h.GetEnvelope)
		r.Post("/records/{id}/envelopes", h.PostEnvelope)
		r.Delete("/records/{id}", h.DeleteRecord)
	})

	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("vault: listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("vault: server error: %v", err)
		}
	}()

	<-quit
	log.Println("vault: shutting down...")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("vault: forced shutdown: %v", err)
	}
	log.Println("vault: stopped")
}

// redactEnvelopeMiddleware is a structured-log guard: it wraps the response
// writer with a no-op body interceptor. Actual redaction of envelope_ciphertext
// from server logs is handled by never logging response bodies in the handler layer
// and using middleware.Logger which logs only method/path/status/duration.
// This middleware documents the security contract at the routing level.
func redactEnvelopeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The chi Logger middleware does not log response bodies.
		// This middleware is a documentation-layer guard — the real enforcement
		// is that handlers never pass envelope_ciphertext to log.Printf/log.Println.
		next.ServeHTTP(w, r)
	})
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("vault: required env var %s is not set", key)
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
