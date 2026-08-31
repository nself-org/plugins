// functions-v8 is the edge functions plugin service for nSelf.
// It manages a pool of Deno isolates for running short-lived TypeScript functions.
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nself-org/nself-functions-v8/internal/api"
	"github.com/nself-org/nself-functions-v8/internal/config"
	"github.com/nself-org/nself-functions-v8/internal/deploy"
	"github.com/nself-org/nself-functions-v8/internal/isolate"
	"github.com/nself-org/nself-functions-v8/internal/logs"
	"github.com/nself-org/nself-functions-v8/internal/store"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("config: %v", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	// Connect to database.
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database connect: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("database ping: %v", err)
	}
	logger.Info("database connected")

	// Build component instances.
	st := store.New(pool)
	isoPool := isolate.NewPool(cfg.DenoBinaryPath, cfg.PoolSize, cfg.MaxMemoryMB, cfg.MaxDurationMS, logger)
	lgr := logs.New(pool, logger)
	checker := deploy.NewTypeChecker(cfg.DenoBinaryPath)
	h := api.New(st, isoPool, lgr, logger, checker)

	// Wire router.
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(35 * time.Second))
	api.RegisterRoutes(r, h)

	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  35 * time.Second,
		WriteTimeout: 40 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	logger.Info("functions-v8 starting", "addr", addr, "pool_size", cfg.PoolSize)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "err", err)
	}
}
