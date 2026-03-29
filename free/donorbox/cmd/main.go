package main

import (
	"log"
	"os"
	"strconv"

	"github.com/nself-org/nself-donorbox/internal"
	sdk "github.com/nself-org/plugin-sdk"
)

func main() {
	port := 3074
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("[nself-donorbox] DATABASE_URL is required")
	}

	pool, err := sdk.ConnectDB(dbURL)
	if err != nil {
		log.Fatalf("[nself-donorbox] database connection failed: %v", err)
	}
	defer pool.Close()

	db := internal.NewDB(pool)
	if err := db.InitSchema(); err != nil {
		log.Fatalf("[nself-donorbox] schema init failed: %v", err)
	}

	apiKey := os.Getenv("DONORBOX_API_KEY")
	email := os.Getenv("DONORBOX_EMAIL")
	webhookSecret := os.Getenv("DONORBOX_WEBHOOK_SECRET")

	var client *internal.DonorboxClient
	if apiKey != "" && email != "" {
		client = internal.NewDonorboxClient(email, apiKey)
	}

	srv := sdk.NewServer(port)
	r := srv.Router()

	r.Use(sdk.Recovery)
	r.Use(sdk.Logger)
	r.Use(sdk.CORS)
	r.Use(sdk.RequestID)

	internal.RegisterRoutes(r, db, client, webhookSecret)

	log.Printf("[nself-donorbox] starting on port %d", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("[nself-donorbox] server error: %v", err)
	}
}
