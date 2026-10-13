package main

import (
	"context"
	"log"
	"os"

	sdk "github.com/nself-org/plugin-sdk"

	"github.com/nself-org/nself-backup/internal"
)

// Size-cap exception: plugin entry-point main() — 68L startup wiring (env/db/router/server); single invocation, not a reusable unit.
func main() {
	cfg := sdk.LoadConfig()
	if cfg.DatabaseURL == "" {
		cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	}
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	// Override port to 3050 unless PORT env is set.
	port := 3050
	if cfg.Port != 3000 {
		port = cfg.Port
	}

	pool, err := sdk.ConnectDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	store := internal.NewStore(pool)

	internal.BootMigrations(context.Background(), pool)

	storagePath := os.Getenv("BACKUP_STORAGE_PATH")
	if storagePath == "" {
		storagePath = "/tmp/nself-backups"
	}

	pgDumpPath := os.Getenv("BACKUP_PG_DUMP_PATH")
	if pgDumpPath == "" {
		pgDumpPath = "pg_dump"
	}

	pgRestorePath := os.Getenv("BACKUP_PG_RESTORE_PATH")
	if pgRestorePath == "" {
		pgRestorePath = "pg_restore"
	}

	h := internal.NewHandler(store, cfg.DatabaseURL, storagePath, pgDumpPath, pgRestorePath)

	srv := sdk.NewServer(port)
	r := srv.Router()

	r.Use(sdk.Recovery)
	r.Use(sdk.CORS)
	r.Use(sdk.RequestID)
	r.Use(sdk.Logger)

	// Own /health so it can report the boot migration counts (sdk only adds a plain liveness default).
	r.Get("/health", internal.HealthHandler)
	r.Head("/health", internal.HealthHandler)

	r.Post("/v1/backups", h.CreateBackup)
	r.Get("/v1/backups", h.ListBackups)
	r.Get("/v1/backups/{id}", h.GetBackup)
	r.Delete("/v1/backups/{id}", h.DeleteBackup)
	r.Post("/v1/restore", h.CreateRestore)

	// Schedule endpoints.
	r.Post("/v1/schedules", h.CreateSchedule)
	r.Get("/v1/schedules", h.ListSchedules)

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
