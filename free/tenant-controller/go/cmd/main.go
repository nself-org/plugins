// tenant-controller is the nCloud multi-tenant master controller daemon.
// It manages N isolated nSelf project instances behind one nCloud deploy.
//
// Feature flag: NSELF_FLAG_MULTI_TENANT_CONTROLLER=true (required to start).
// When the flag is OFF, all nself project * commands return 503.
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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-tenant-controller/internal/config"
	"github.com/nself-org/nself-tenant-controller/internal/db"
	"github.com/nself-org/nself-tenant-controller/internal/server"
)

func main() {
	setupLogger()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config error", "err", err)
		os.Exit(1)
	}

	// Feature flag gate: NSELF_FLAG_MULTI_TENANT_CONTROLLER must be true.
	if !cfg.Enabled {
		slog.Error("multi-tenant controller not enabled",
			"hint", "Set NSELF_FLAG_MULTI_TENANT_CONTROLLER=true or NSELF_CONTROLLER_ENABLED=true")
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	// Connect to Postgres (superuser / controller role).
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database connection failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Run embedded migrations.
	if err := db.Migrate(ctx, pool); err != nil {
		slog.Error("migration failed", "err", err)
		os.Exit(1)
	}

	// Warn when project count is at or above 80% of cap.
	go tenantCountWatcher(ctx, pool, cfg)

	// Start HTTP server.
	handler := server.New(cfg, pool)
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("tenant-controller listening", "addr", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "err", err)
			cancel()
		}
	}()

	// Wait for shutdown signal.
	<-ctx.Done()
	slog.Info("shutting down tenant-controller")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	if err := httpServer.Shutdown(shutCtx); err != nil {
		slog.Error("graceful shutdown failed", "err", err)
	}

	slog.Info("tenant-controller stopped")
}

// tenantCountWatcher emits an alert when active project count exceeds 80% of max.
func tenantCountWatcher(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	threshold := int(float64(cfg.MaxProjects) * 0.8)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var count int
			_ = pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM nself_controller.projects WHERE status='active' AND deleted_at IS NULL`,
			).Scan(&count)
			if count >= threshold {
				slog.Warn("tenant count near cap",
					"active", count,
					"max", cfg.MaxProjects,
					"threshold_pct", 80,
				)
			}
			slog.Debug("tenant count check", "active", count, "threshold", threshold)
		}
	}
}

// setupLogger configures structured JSON logging.
func setupLogger() {
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})))
}
