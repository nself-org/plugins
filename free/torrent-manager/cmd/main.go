package main

import (
	"log"
	"os"
	"strconv"

	sdk "github.com/nself-org/plugin-sdk"

	"github.com/nself-org/nself-torrent-manager/internal"
)

func main() {
	port := 3201
	if v := os.Getenv("TORRENT_MANAGER_PORT"); v != "" {
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
		log.Fatal("torrent-manager: DATABASE_URL is required")
	}

	pool, err := sdk.ConnectDB(dbURL)
	if err != nil {
		log.Fatalf("torrent-manager: %v", err)
	}
	defer pool.Close()

	cfg := internal.LoadConfig()
	db := internal.NewDB(pool)
	if err := db.InitSchema(); err != nil {
		log.Fatalf("torrent-manager: schema init failed: %v", err)
	}

	srv := sdk.NewServer(port)
	r := srv.Router()

	r.Use(sdk.CORS)
	r.Use(sdk.Logger)
	r.Use(sdk.Recovery)
	r.Use(sdk.RequestID)

	internal.RegisterRoutes(r, db, cfg)

	log.Printf("torrent-manager: starting on port %d", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("torrent-manager: server error: %v", err)
	}
}
