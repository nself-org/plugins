package main

import (
	"log"
	"os"
	"strconv"

	"github.com/nself-org/nself-shopify/internal"
	sdk "github.com/nself-org/plugin-sdk"
)

func main() {
	port := 3072
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("[nself-shopify] DATABASE_URL is required")
	}

	accessToken := os.Getenv("SHOPIFY_ACCESS_TOKEN")
	if accessToken == "" {
		log.Fatal("[nself-shopify] SHOPIFY_ACCESS_TOKEN is required")
	}

	shopDomain := os.Getenv("SHOPIFY_SHOP_DOMAIN")
	if shopDomain == "" {
		log.Fatal("[nself-shopify] SHOPIFY_SHOP_DOMAIN is required")
	}

	pool, err := sdk.ConnectDB(dbURL)
	if err != nil {
		log.Fatalf("[nself-shopify] database connection failed: %v", err)
	}
	defer pool.Close()

	if err := internal.Migrate(pool); err != nil {
		log.Fatalf("[nself-shopify] migration failed: %v", err)
	}

	cfg := internal.LoadConfig()

	srv := sdk.NewServer(port)
	r := srv.Router()

	r.Use(sdk.Recovery)
	r.Use(sdk.Logger)
	r.Use(sdk.CORS)
	r.Use(sdk.RequestID)

	internal.RegisterRoutes(r, pool, cfg)

	log.Printf("[nself-shopify] starting on port %d", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("[nself-shopify] server error: %v", err)
	}
}
