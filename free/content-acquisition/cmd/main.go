package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/nself-org/nself-content-acquisition/internal"
	sdk "github.com/nself-org/plugin-sdk"
)

func main() {
	port := 3202
	if v := os.Getenv("CONTENT_ACQUISITION_PORT"); v != "" {
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
		log.Fatal("content-acquisition: DATABASE_URL is required")
	}

	pool, err := sdk.ConnectDB(dbURL)
	if err != nil {
		log.Fatalf("content-acquisition: %v", err)
	}
	defer pool.Close()

	internal.MustBootMigrations(context.Background(), pool)

	db := internal.NewDB(pool)
	if err := db.InitSchema(); err != nil {
		log.Fatalf("content-acquisition: schema init failed: %v", err)
	}

	srv := sdk.NewServer(port)
	r := srv.Router()
	r.Use(sdk.CORS)
	r.Use(sdk.Logger)
	r.Use(sdk.Recovery)
	r.Use(sdk.RequestID)
	r.Get("/health", internal.BootHealthHandler(pool))

	internal.RegisterRoutes(r, db)

	log.Printf("content-acquisition: starting on port %d", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("content-acquisition: server error: %v", err)
	}
}
