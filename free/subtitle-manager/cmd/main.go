package main

import (
	"log"
	"os"
	"strconv"

	sdk "github.com/nself-org/plugin-sdk"
	"github.com/nself-org/nself-subtitle-manager/internal"
)

func main() {
	port := 3204
	if v := os.Getenv("SUBTITLE_MANAGER_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	} else if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("subtitle-manager: DATABASE_URL is required")
	}

	pool, err := sdk.ConnectDB(dbURL)
	if err != nil {
		log.Fatalf("subtitle-manager: %v", err)
	}
	defer pool.Close()

	cfg := internal.LoadConfig()
	db := internal.NewDB(pool)
	if err := db.InitSchema(); err != nil {
		log.Fatalf("subtitle-manager: schema init failed: %v", err)
	}

	srv := sdk.NewServer(port)
	r := srv.Router()
	r.Use(sdk.CORS)
	r.Use(sdk.Logger)
	r.Use(sdk.Recovery)
	r.Use(sdk.RequestID)

	osClient := internal.NewOpenSubtitlesClient(cfg.OpenSubtitlesKey)
	internal.RegisterRoutes(r, db, cfg, osClient)

	log.Printf("subtitle-manager: starting on port %d", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("subtitle-manager: server error: %v", err)
	}
}
