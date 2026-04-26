package main

import (
	"context"
	"log"

	sdk "github.com/nself-org/plugin-sdk"

	"github.com/nself-org/nself-push/internal"
)

func main() {
	cfg := internal.LoadConfig()

	if err := cfg.Validate(); err != nil {
		log.Fatalf("[push] configuration error: %v", err)
	}

	// Database connection.
	dbURL := cfg.RedisURL // RedisURL stored separately; DB URL via sdk.LoadConfig
	sdkCfg := sdk.LoadConfig()
	if sdkCfg.DatabaseURL == "" {
		log.Fatal("[push] DATABASE_URL is required")
	}
	if cfg.Port == 3000 {
		cfg.Port = 3053
	}
	_ = dbURL // redis URL used by dispatcher internals only

	pool, err := sdk.ConnectDB(sdkCfg.DatabaseURL)
	if err != nil {
		log.Fatalf("[push] database connection failed: %v", err)
	}
	defer pool.Close()

	// Run schema migrations (idempotent).
	if err := internal.Migrate(pool); err != nil {
		log.Fatalf("[push] migration failed: %v", err)
	}

	ctx := context.Background()

	// Build APNs client (nil if not configured — plugin starts in degraded mode).
	apnsClient, err := internal.NewAPNsClient(cfg)
	if err != nil {
		log.Fatalf("[push] APNs client init failed: %v", err)
	}
	if apnsClient != nil {
		log.Printf("[push] APNs enabled (bundle: %s, sandbox: %v)", cfg.APNsBundleID, cfg.APNsSandbox)
	} else {
		log.Printf("[push] APNs not configured — PUSH_APNS_* env vars unset; iOS delivery unavailable")
	}

	// Build FCM client (nil if not configured).
	fcmClient, err := internal.NewFCMClient(ctx, cfg)
	if err != nil {
		log.Fatalf("[push] FCM client init failed: %v", err)
	}
	if fcmClient != nil {
		log.Printf("[push] FCM enabled (project: %s)", cfg.FCMProjectID)
	} else {
		log.Printf("[push] FCM not configured — PUSH_FCM_* env vars unset; Android delivery unavailable")
	}

	if apnsClient == nil && fcmClient == nil {
		log.Printf("[push] WARNING: neither APNs nor FCM is configured — push service will accept and reject all dispatch requests. Set PUSH_APNS_* or PUSH_FCM_* to enable delivery.")
	}

	dispatcher := internal.NewDispatcher(pool, apnsClient, fcmClient, cfg)

	srv := sdk.NewServer(cfg.Port)
	r := srv.Router()

	r.Use(sdk.CORS)
	r.Use(sdk.Logger)
	r.Use(sdk.Recovery)

	internal.RegisterRoutes(r, pool, dispatcher)

	log.Printf("[push] starting on port %d", cfg.Port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("[push] server error: %v", err)
	}
}
