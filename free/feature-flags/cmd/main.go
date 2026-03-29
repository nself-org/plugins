package main

import (
	"log"
	"os"
	"strconv"

	sdk "github.com/nself-org/plugin-sdk"

	"github.com/nself-org/nself-feature-flags/internal"
)

func main() {
	port := 3305
	if v := os.Getenv("FF_PLUGIN_PORT"); v != "" {
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
		log.Fatal("feature-flags: DATABASE_URL is required")
	}

	pool, err := sdk.ConnectDB(dbURL)
	if err != nil {
		log.Fatalf("feature-flags: %v", err)
	}
	defer pool.Close()

	db := internal.NewDB(pool)
	if err := db.InitSchema(); err != nil {
		log.Fatalf("feature-flags: schema init failed: %v", err)
	}

	srv := sdk.NewServer(port)
	r := srv.Router()

	r.Use(sdk.CORS)
	r.Use(sdk.Logger)
	r.Use(sdk.Recovery)
	r.Use(sdk.RequestID)

	internal.RegisterRoutes(r, db)

	log.Printf("feature-flags: starting on port %d", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("feature-flags: server error: %v", err)
	}
}
