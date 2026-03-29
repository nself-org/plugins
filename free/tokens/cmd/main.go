package main

import (
	"log"
	"os"
	"strconv"

	sdk "github.com/nself-org/plugin-sdk"

	"github.com/nself-org/nself-tokens/internal"
)

func main() {
	port := 3107
	if v := os.Getenv("TOKENS_PLUGIN_PORT"); v != "" {
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
		log.Fatal("tokens: DATABASE_URL is required")
	}

	encKey := os.Getenv("TOKENS_ENCRYPTION_KEY")
	if encKey == "" {
		log.Fatal("tokens: TOKENS_ENCRYPTION_KEY is required")
	}

	pool, err := sdk.ConnectDB(dbURL)
	if err != nil {
		log.Fatalf("tokens: %v", err)
	}
	defer pool.Close()

	cfg := internal.LoadConfig()
	db := internal.NewDB(pool)
	if err := db.InitSchema(); err != nil {
		log.Fatalf("tokens: schema init failed: %v", err)
	}

	srv := sdk.NewServer(port)
	r := srv.Router()

	r.Use(sdk.CORS)
	r.Use(sdk.Logger)
	r.Use(sdk.Recovery)
	r.Use(sdk.RequestID)

	internal.RegisterRoutes(r, db, cfg)

	log.Printf("tokens: starting on port %d", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("tokens: server error: %v", err)
	}
}
