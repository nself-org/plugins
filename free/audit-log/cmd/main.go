package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/nself-org/plugins/free/audit-log/internal"
	sdk "github.com/nself-org/plugin-sdk"
)

func main() {
	port := 3308
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("[audit-log] DATABASE_URL is required")
	}

	pool, err := sdk.ConnectDB(dbURL)
	if err != nil {
		log.Fatalf("[audit-log] database connection failed: %v", err)
	}
	defer pool.Close()

	if err := internal.Migrate(pool); err != nil {
		log.Fatalf("[audit-log] migration failed: %v", err)
	}

	secret := os.Getenv("PLUGIN_INTERNAL_SECRET")
	if secret == "" {
		log.Fatal("[audit-log] PLUGIN_INTERNAL_SECRET is required; refusing to start with unauthenticated audit endpoints")
	}

	// HASURA_GRAPHQL_ADMIN_SECRET is optional. When set, GET /admin/events also
	// accepts requests carrying this header so the Admin UI can query the log
	// via Hasura's standard admin header without exposing the plugin secret.
	adminSecret := os.Getenv("HASURA_GRAPHQL_ADMIN_SECRET")

	r := chi.NewRouter()
	r.Use(sdk.Recovery)
	r.Use(sdk.Logger)
	r.Use(sdk.CORS)
	r.Use(sdk.RequestID)

	// RegisterRoutes mounts /health plus all plugin-specific endpoints.
	// We build the router directly (rather than via sdk.NewServer) so that
	// our plugin-level /health handler — which checks DB connectivity — takes
	// precedence over the SDK's generic one.
	internal.RegisterRoutes(r, pool, secret, adminSecret)

	r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	addr := net.JoinHostPort("", strconv.Itoa(port))
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("[audit-log] starting on port %d", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	select {
	case sig := <-quit:
		log.Printf("[audit-log] received %s, shutting down", sig)
	case err := <-errCh:
		log.Fatalf("[audit-log] server error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("[audit-log] shutdown error: %v", err)
	}
}
