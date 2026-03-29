package main

import (
	"log"
	"os"
	"strconv"

	"github.com/nself-org/nself-webhooks/internal"
	sdk "github.com/nself-org/plugin-sdk"
)

func main() {
	port := 3060
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("[nself-webhooks] DATABASE_URL is required")
	}

	pool, err := sdk.ConnectDB(dbURL)
	if err != nil {
		log.Fatalf("[nself-webhooks] database connection failed: %v", err)
	}
	defer pool.Close()

	if err := internal.Migrate(pool); err != nil {
		log.Fatalf("[nself-webhooks] migration failed: %v", err)
	}

	dispatcher := internal.NewDispatcher(pool)
	go dispatcher.StartProcessing()

	srv := sdk.NewServer(port)
	r := srv.Router()

	r.Use(sdk.Recovery)
	r.Use(sdk.Logger)
	r.Use(sdk.CORS)
	r.Use(sdk.RequestID)

	internal.RegisterRoutes(r, pool, dispatcher)

	log.Printf("[nself-webhooks] starting on port %d", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("[nself-webhooks] server error: %v", err)
	}
}
