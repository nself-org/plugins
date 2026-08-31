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
	"github.com/nself-org/nself-podcast/internal"
)

func main() {
	cfg := internal.LoadConfig()
	ctx := context.Background()
	pool, err := internal.NewDB(ctx, cfg.DatabaseURL)
	if err != nil { log.Fatalf("db: %v", err) }
	defer pool.Close()
	if err := internal.InitSchema(ctx, pool); err != nil {
		log.Fatalf("schema init: %v", err)
	}
	h := internal.NewHandlers(pool)
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Get("/health", h.Health)
	r.Get("/ready", h.Ready)
	r.Route("/api/v1/podcasts", func(r chi.Router) {
		r.Get("/", h.ListPodcasts)
		r.Post("/", h.CreatePodcast)
		r.Get("/{id}", h.GetPodcast)
		r.Put("/{id}", h.UpdatePodcast)
		r.Delete("/{id}", h.DeletePodcast)
		r.Get("/{id}/episodes", h.ListEpisodes)
		r.Post("/{id}/episodes", h.CreateEpisode)
		r.Get("/{id}/feed.xml", h.GetFeed)
		r.Post("/{id}/subscribe", h.Subscribe)
		r.Delete("/{id}/unsubscribe", h.Unsubscribe)
	})
	r.Route("/api/v1/episodes", func(r chi.Router) {
		r.Get("/{id}", h.GetEpisode)
		r.Put("/{id}", h.UpdateEpisode)
		r.Delete("/{id}", h.DeleteEpisode)
		r.Post("/{id}/play", h.RecordPlay)
	})
	srv := &http.Server{Addr: fmt.Sprintf(":%s", cfg.Port), Handler: r, ReadTimeout: 30*time.Second, WriteTimeout: 30*time.Second}
	go func() {
		log.Printf("podcast plugin on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed { log.Fatalf("listen: %v", err) }
	}()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}
