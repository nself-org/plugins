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
	"github.com/nself-org/nself-home/internal"
)

func main() {
	cfg := internal.LoadConfig()

	ctx := context.Background()
	pool, err := internal.NewDB(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	if err := internal.InitSchema(ctx, pool); err != nil {
		log.Fatalf("schema init: %v", err)
	}

	h := internal.NewHandlers(pool, cfg)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/health", h.Health)
	r.Get("/ready", h.Ready)

	r.Post("/home/command", h.HandleCommand)
	r.Get("/home/devices", h.HandleListDevices)
	r.Get("/home/state/{entity_id}", h.HandleGetState)
	r.Post("/home/scene/{scene_id}", h.HandleActivateScene)
	r.Get("/home/automations", h.HandleListAutomations)
	r.Post("/home/automations/{id}/toggle", h.HandleToggleAutomation)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("nself-home listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
}
