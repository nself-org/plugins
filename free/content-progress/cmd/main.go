package main

import (
	"log"
	"os"

	sdk "github.com/nself-org/plugin-sdk"
	"github.com/nself-org/nself-content-progress/internal"
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

	db := internal.NewDB(pool, cfg)
	if err := db.InitSchema(); err != nil {
		log.Fatalf("content-progress: schema init failed: %v", err)
	}

	srv := sdk.NewServer(cfg.Port)
	r := srv.Router()

	r.Use(sdk.CORS)
	r.Use(sdk.Logger)
	r.Use(sdk.Recovery)
	r.Use(sdk.RequestID)

	internal.RegisterRoutes(r, db, cfg)

	log.Printf("content-progress: starting on port %d", cfg.Port)
	log.Printf("content-progress: complete threshold: %d%%", cfg.CompleteThreshold)
	log.Printf("content-progress: history sampling: %ds", cfg.HistorySampleSeconds)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("content-progress: server error: %v", err)
	}
}
