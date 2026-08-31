// nself-crdt is the CRDT offline-first plugin for nSelf.
// It provides a self-hosted Yjs (y-websocket protocol) and automerge sync server
// backed by Postgres. Drop-in replacement for Liveblocks/PartyKit.
package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-crdt/internal/api"
	"github.com/nself-org/nself-crdt/internal/automerge"
	"github.com/nself-org/nself-crdt/internal/store"
	"github.com/nself-org/nself-crdt/internal/yjs"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slogLevel(),
	}))

	dbURL := mustEnv("DATABASE_URL")

	maxDocSizeMB := envInt("CRDT_MAX_DOC_SIZE_MB", 10)
	retentionDays := envInt("CRDT_RETENTION_DAYS", 90)
	port := envStr("CRDT_PORT", "8211")

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	// Connect to Postgres.
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("crdt: database connect failed: %v", err)
	}
	defer pool.Close()

	// Run migrations.
	if err := store.Migrate(ctx, pool); err != nil {
		log.Fatalf("crdt: migrate failed: %v", err)
	}
	logger.Info("crdt: migrations applied")

	st := store.New(pool)
	hub := yjs.NewAwarenessHub()

	yjsHandler := yjs.NewHandler(st, hub, maxDocSizeMB, logger)
	amHandler := automerge.NewSyncHandler(st, maxDocSizeMB, logger)
	comp := automerge.NewCompactor(st, retentionDays, logger)

	// Start background compactor.
	go comp.Run(ctx)

	// Build HTTP router.
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	handlers := api.New(st, yjsHandler, amHandler, comp, logger)
	handlers.Mount(r)

	srv := &http.Server{
		Addr:         "127.0.0.1:" + port,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // WebSocket connections must not have a write deadline
		IdleTimeout:  120 * time.Second,
	}

	logger.Info("crdt: listening", "addr", srv.Addr)

	go func() {
		<-ctx.Done()
		shutCtx, sc := context.WithTimeout(context.Background(), 15*time.Second)
		defer sc()
		_ = srv.Shutdown(shutCtx)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("crdt: server error: %v", err)
	}
	logger.Info("crdt: shutdown complete")
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("crdt: required env var %s is not set", key)
	}
	return v
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func slogLevel() slog.Level {
	switch os.Getenv("LOG_LEVEL") {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
