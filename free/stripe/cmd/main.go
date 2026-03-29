package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-stripe/internal"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("[stripe] Starting nSelf Stripe plugin...")

	// Load configuration from environment
	cfg, err := internal.LoadConfig()
	if err != nil {
		log.Fatalf("[stripe] Configuration error: %v", err)
	}

	log.Printf("[stripe] Loaded %d Stripe account(s)", len(cfg.StripeAccounts))
	for _, acc := range cfg.StripeAccounts {
		mode := "LIVE"
		if internal.IsTestMode(acc.APIKey) {
			mode = "TEST"
		}
		hasSecret := "no"
		if acc.WebhookSecret != "" {
			hasSecret = "yes"
		}
		log.Printf("[stripe]   Account %q: mode=%s webhook_secret=%s", acc.ID, mode, hasSecret)
	}

	// Connect to PostgreSQL
	ctx := context.Background()
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("[stripe] Invalid DATABASE_URL: %v", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Fatalf("[stripe] Failed to connect to database: %v", err)
	}
	defer pool.Close()

	// Verify connectivity
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("[stripe] Database ping failed: %v", err)
	}
	log.Println("[stripe] Connected to database")

	// Initialize database schema
	db := internal.NewDB(pool)
	if err := db.InitSchema(ctx); err != nil {
		log.Fatalf("[stripe] Schema initialization failed: %v", err)
	}

	// Create Stripe API client for sync operations
	client := internal.NewStripeClient(cfg.StripeAPIKey)

	// Create HTTP server
	server := &internal.Server{
		Config:   cfg,
		DB:       db,
		Client:   client,
		Accounts: cfg.StripeAccounts,
		StartAt:  time.Now(),
	}

	router := server.NewRouter()
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	httpServer := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("[stripe] Server listening on http://%s", addr)
		log.Printf("[stripe] Webhook endpoint: http://%s/webhooks/stripe", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[stripe] Server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	sig := <-stop
	log.Printf("[stripe] Received signal %v, shutting down...", sig)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("[stripe] Server shutdown error: %v", err)
	}

	pool.Close()
	log.Println("[stripe] Shutdown complete")
}
