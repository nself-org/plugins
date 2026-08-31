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

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/nself-org/nself-linkedin/internal"
)

func main() {
	cfg, err := internal.LoadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()
	rawPool, err := internal.NewDB(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer rawPool.Close()

	if err := internal.InitSchema(ctx, rawPool); err != nil {
		log.Fatalf("schema init: %v", err)
	}

	pool := internal.NewPgPool(rawPool)

	h := &internal.Handlers{
		Cfg:  cfg,
		Pool: pool,
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)

	// Health — unauthenticated.
	r.Get("/health", h.HandleHealth)

	// OAuth flow — unauthenticated (browser redirects).
	r.Get("/linkedin/auth/start", h.HandleAuthStart)
	r.Get("/linkedin/auth/callback", h.HandleAuthCallback)

	// Protected routes — require X-Internal-Token.
	r.Group(func(r chi.Router) {
		r.Use(internal.InternalAuthMiddleware(cfg.InternalSecret))
		r.Get("/linkedin/status", h.HandleStatus)
		r.Post("/linkedin/publish", h.HandlePublish)
		r.Get("/linkedin/posts", h.HandleListPosts)
		r.Delete("/linkedin/auth", h.HandleDisconnect)
		r.Get("/internal/tools", h.HandleTools)
	})

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("nself-linkedin listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-quit
	log.Println("nself-linkedin shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server shutdown: %v", err)
	}
	log.Println("nself-linkedin stopped")
}
