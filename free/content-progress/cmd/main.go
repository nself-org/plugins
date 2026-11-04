package main

import (
	"context"
	"log"
	"os"

	"github.com/nself-org/nself-content-progress/internal"
	sdk "github.com/nself-org/plugin-sdk"
)

func main() {
	cfg := internal.LoadConfig()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("content-progress: invalid config: %v", err)
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("content-progress: DATABASE_URL is required")
	}

	pool, err := sdk.ConnectDB(dbURL)
	if err != nil {
		log.Fatalf("content-progress: %v", err)
	}
	defer pool.Close()

	internal.MustBootMigrations(context.Background(), pool)

	// Boot migrations own the whole schema (public tables, D-0832); there is no Go DDL.
	db := internal.NewDB(pool, cfg)

	srv := sdk.NewServer(cfg.Port)
	r := srv.Router()

	r.Use(sdk.CORS)
	r.Use(sdk.Logger)
	r.Use(sdk.Recovery)
	r.Use(sdk.RequestID)
	r.Get("/health", internal.BootHealthHandler(pool))

	internal.RegisterRoutes(r, db, cfg)

	log.Printf("content-progress: starting on port %d", cfg.Port)
	log.Printf("content-progress: complete threshold: %d%%", cfg.CompleteThreshold)
	log.Printf("content-progress: history sampling: %ds", cfg.HistorySampleSeconds)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("content-progress: server error: %v", err)
	}
}
