// Purpose: plugin-pty entry point.
//   Starts HTTP + WebSocket server on port 9100.
//   Provides PTY session lifecycle and WebSocket I/O relay for ClawDE.
// Inputs:  NSELF_DB_URL, NSELF_LICENSE_KEY, PTY_MAX_PER_TENANT, PTY_SESSION_TIMEOUT_SECS.
// Outputs: HTTP JSON + WebSocket PTY I/O.
// Constraints:
//   - NSELF_LICENSE_KEY must be set (pro license gate).
//   - All session operations require source_account_id.
//   - Max PTY sessions per tenant enforced before spawn.
//
// SPORT: F04-PLUGIN-INVENTORY-PRO.md — plugin-pty
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	ptyhandler "github.com/nself-org/plugin-pty/internal/pty"
)

func main() {
	// License gate
	if os.Getenv("NSELF_LICENSE_KEY") == "" {
		fmt.Fprintln(os.Stderr, "plugin-pty: NSELF_LICENSE_KEY is required (pro license)")
		os.Exit(1)
	}

	dbURL := os.Getenv("NSELF_DB_URL")
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "plugin-pty: NSELF_DB_URL is required")
		os.Exit(1)
	}

	maxPerTenant := 5
	if v := os.Getenv("PTY_MAX_PER_TENANT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxPerTenant = n
		}
	}

	sessionTTL := 3600 * time.Second
	if v := os.Getenv("PTY_SESSION_TIMEOUT_SECS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			sessionTTL = time.Duration(n) * time.Second
		}
	}

	port := os.Getenv("PTY_PORT")
	if port == "" {
		port = "9100"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "plugin-pty: db connect error: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	cfg := ptyhandler.Config{
		MaxPerTenant: maxPerTenant,
		SessionTTL:   sessionTTL,
	}

	h := ptyhandler.NewHandler(pool, cfg)
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	h.Routes(r)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", port),
		Handler: r,
	}

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
		<-quit
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	fmt.Printf("plugin-pty listening on :%s\n", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "plugin-pty: server error: %v\n", err)
		os.Exit(1)
	}
}
