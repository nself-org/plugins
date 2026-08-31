// Command job-queue is the nSelf job-queue plugin service.
//
// Purpose: Durable background job queue with Redis execution + Postgres visibility.
// Inputs: REDIS_URL, DATABASE_URL, JOBQUEUE_* env vars.
// Outputs: HTTP API on JOBQUEUE_PORT (default 8213); worker goroutines per queue.
// Constraints:
//   - ctx.License.Valid() checked at startup: NSELF_JOB_QUEUE env var must be true.
//   - Custom service slot CS_10.
//   - source_account_id scopes all np_jobs/np_job_dlq queries (multi-tenant isolation).
//   - Unregistered job types fail fast → retry/DLQ path (never silent completion).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nself-org/nself-job-queue/api"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// License gate: ctx.License.Valid() — NSELF_JOB_QUEUE must be "true".
	if os.Getenv("NSELF_JOB_QUEUE") != "true" {
		slog.Error("license check failed: NSELF_JOB_QUEUE is not set to true; ɳSelf+ license required")
		os.Exit(1)
	}

	cfg := loadConfig()

	srv, err := api.NewServer(cfg, logger)
	if err != nil {
		slog.Error("failed to create server", "err", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := srv.StartWorkers(ctx); err != nil {
		slog.Error("failed to start workers", "err", err)
		os.Exit(1)
	}

	httpServer := &http.Server{
		Addr:         fmt.Sprintf("127.0.0.1:%s", cfg.Port),
		Handler:      srv.Router(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	slog.Info("job-queue service starting", "port", cfg.Port, "queues", cfg.Queues)

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "err", err)
			cancel()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down job-queue service")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	httpServer.Shutdown(shutdownCtx)
}

func loadConfig() api.Config {
	return api.Config{
		RedisURL:    getenv("REDIS_URL", "redis://127.0.0.1:6379"),
		DatabaseURL: getenv("DATABASE_URL", ""),
		Port:        getenv("JOBQUEUE_PORT", "8213"),
		Concurrency: getenvInt("JOBQUEUE_CONCURRENCY", 5),
		MaxAttempts: getenvInt("JOBQUEUE_MAX_ATTEMPTS", 8),
		Queues:      splitCSV(getenv("JOBQUEUE_QUEUES", "default,email,ai,media")),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range splitOn(s, ',') {
		if trimmed := trim(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func splitOn(s string, sep rune) []string {
	var parts []string
	start := 0
	for i, c := range s {
		if c == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
