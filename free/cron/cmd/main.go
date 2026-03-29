package main

import (
	"log"

	sdk "github.com/nself-org/plugin-sdk"

	"github.com/nself-org/nself-cron/internal"
)

func main() {
	cfg := sdk.LoadConfig()
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if cfg.Port == 3000 {
		cfg.Port = 3051
	}

	pool, err := sdk.ConnectDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer pool.Close()

	if err := internal.Migrate(pool); err != nil {
		log.Fatalf("database migration failed: %v", err)
	}

	app := internal.NewApp(pool, cfg)

	// Recover missed jobs on startup.
	app.RecoverMissed()

	// Start background scheduler goroutine.
	app.StartScheduler()

	// Start daily history retention cleanup.
	app.StartRetentionCleanup()

	srv := sdk.NewServer(cfg.Port)
	r := srv.Router()

	r.Use(sdk.CORS)
	r.Use(sdk.Logger)
	r.Use(sdk.Recovery)

	internal.RegisterRoutes(r, app)

	log.Printf("nself-cron listening on :%d", cfg.Port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
