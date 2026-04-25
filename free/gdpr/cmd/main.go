package main

import (
	"context"
	"database/sql"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/nself-org/plugins/free/gdpr/internal"
	_ "github.com/lib/pq"
)

func main() {
	port := 3319
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("[gdpr] DATABASE_URL is required")
	}

	secret := os.Getenv("PLUGIN_INTERNAL_SECRET")
	if secret == "" {
		log.Fatal("[gdpr] PLUGIN_INTERNAL_SECRET is required")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("[gdpr] database open failed: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	svc := internal.NewService(db)
	if err := svc.Migrate(ctx); err != nil {
		cancel()
		log.Fatalf("[gdpr] migration failed: %v", err)
	}
	cancel()

	handler := internal.Handler(secret, svc)
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("[gdpr] listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[gdpr] server error: %v", err)
		}
	}()

	<-stop
	log.Println("[gdpr] shutting down")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("[gdpr] shutdown error: %v", err)
	}
}
