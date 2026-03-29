package main

import (
	"log"
	"os"
	"strconv"

	"github.com/nself-org/nself-paypal/internal"
	sdk "github.com/nself-org/plugin-sdk"
)

func main() {
	port := 3071
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("[nself-paypal] DATABASE_URL is required")
	}

	clientID := os.Getenv("PAYPAL_CLIENT_ID")
	if clientID == "" {
		log.Fatal("[nself-paypal] PAYPAL_CLIENT_ID is required")
	}

	clientSecret := os.Getenv("PAYPAL_CLIENT_SECRET")
	if clientSecret == "" {
		log.Fatal("[nself-paypal] PAYPAL_CLIENT_SECRET is required")
	}

	pool, err := sdk.ConnectDB(dbURL)
	if err != nil {
		log.Fatalf("[nself-paypal] database connection failed: %v", err)
	}
	defer pool.Close()

	if err := internal.Migrate(pool); err != nil {
		log.Fatalf("[nself-paypal] migration failed: %v", err)
	}

	cfg := internal.LoadConfig()

	srv := sdk.NewServer(port)
	r := srv.Router()

	r.Use(sdk.Recovery)
	r.Use(sdk.Logger)
	r.Use(sdk.CORS)
	r.Use(sdk.RequestID)

	internal.RegisterRoutes(r, pool, cfg)

	log.Printf("[nself-paypal] starting on port %d", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("[nself-paypal] server error: %v", err)
	}
}
