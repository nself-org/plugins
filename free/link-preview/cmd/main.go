package main

import (
	"log"
	"os"
	"strconv"

	"github.com/nself-org/nself-link-preview/internal"
	sdk "github.com/nself-org/plugin-sdk"
)

func main() {
	port := 3718
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("[nself-link-preview] DATABASE_URL is required")
	}

	pool, err := sdk.ConnectDB(dbURL)
	if err != nil {
		log.Fatalf("[nself-link-preview] database connection failed: %v", err)
	}
	defer pool.Close()

	if err := internal.Migrate(pool); err != nil {
		log.Fatalf("[nself-link-preview] migration failed: %v", err)
	}

	srv := sdk.NewServer(port)
	r := srv.Router()

	r.Use(sdk.Recovery)
	r.Use(sdk.Logger)
	r.Use(sdk.CORS)
	r.Use(sdk.RequestID)

	internal.RegisterRoutes(r, pool)

	log.Printf("[nself-link-preview] starting on port %d", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("[nself-link-preview] server error: %v", err)
	}
}
