package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-github/internal"
)

func main() {
	port := 3002
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("[nself-github] DATABASE_URL is required")
	}

	cfg, err := internal.LoadConfig()
	if err != nil {
		log.Fatalf("[nself-github] config error: %v", err)
	}

	pool, err := connectDB(dbURL)
	if err != nil {
		log.Fatalf("[nself-github] database connection failed: %v", err)
	}
	defer pool.Close()

	if err := internal.Migrate(pool); err != nil {
		log.Fatalf("[nself-github] migration failed: %v", err)
	}

	srv := internal.NewServer(pool, cfg)
	handler := srv.Router()

	addr := net.JoinHostPort("", strconv.Itoa(port))
	httpSrv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("[nself-github] starting on port %d", port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	select {
	case sig := <-quit:
		log.Printf("[nself-github] received %s, shutting down", sig)
	case err := <-errCh:
		log.Fatalf("[nself-github] server error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Fatalf("[nself-github] shutdown error: %v", err)
	}
	log.Println("[nself-github] stopped")
}

// connectDB connects to PostgreSQL with retry logic.
func connectDB(databaseURL string) (*pgxpool.Pool, error) {
	const maxAttempts = 3
	const backoff = 2 * time.Second

	var pool *pgxpool.Pool
	var err error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		pool, err = pgxpool.New(ctx, databaseURL)
		if err != nil {
			cancel()
			log.Printf("[nself-github] db connect attempt %d/%d failed: %v", attempt, maxAttempts, err)
			if attempt < maxAttempts {
				time.Sleep(backoff)
			}
			continue
		}
		err = pool.Ping(ctx)
		cancel()
		if err != nil {
			pool.Close()
			log.Printf("[nself-github] db ping attempt %d/%d failed: %v", attempt, maxAttempts, err)
			if attempt < maxAttempts {
				time.Sleep(backoff)
			}
			continue
		}
		log.Println("[nself-github] connected to database")
		return pool, nil
	}
	return nil, err
}
