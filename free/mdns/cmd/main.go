package main

import (
	"log"
	"os"
	"strconv"

	"github.com/nself-org/nself-mdns/internal"
	sdk "github.com/nself-org/plugin-sdk"
)

func main() {
	port := 3216
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("[nself-mdns] DATABASE_URL is required")
	}

	pool, err := sdk.ConnectDB(dbURL)
	if err != nil {
		log.Fatalf("[nself-mdns] database connection failed: %v", err)
	}
	defer pool.Close()

	db := internal.NewDB(pool)
	if err := db.InitSchema(); err != nil {
		log.Fatalf("[nself-mdns] schema init failed: %v", err)
	}

	srv := sdk.NewServer(port)
	r := srv.Router()

	r.Use(sdk.Recovery)
	r.Use(sdk.Logger)
	r.Use(sdk.CORS)
	r.Use(sdk.RequestID)

	internal.RegisterRoutes(r, db)

	log.Printf("[nself-mdns] starting on port %d", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("[nself-mdns] server error: %v", err)
	}
}
