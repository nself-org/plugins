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
	"github.com/nself-org/nself-tmdb/internal"
)

// newRouter builds the chi router with all tmdb routes wired to h.
// Extracted from main() so route wiring is unit-testable via httptest without
// starting a real listener or touching a live database.
func newRouter(h *internal.Handlers) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/health", h.Health)
	r.Get("/ready", h.Ready)

	r.Route("/api/v1/movies", func(r chi.Router) {
		r.Get("/", h.ListMovies)
		r.Post("/", h.CreateMovie)
		r.Get("/{id}", h.GetMovie)
		r.Put("/{id}", h.UpdateMovie)
		r.Delete("/{id}", h.DeleteMovie)
	})

	r.Route("/api/v1/tv-shows", func(r chi.Router) {
		r.Get("/", h.ListTVShows)
		r.Post("/", h.CreateTVShow)
		r.Get("/{id}", h.GetTVShow)
		r.Put("/{id}", h.UpdateTVShow)
		r.Delete("/{id}", h.DeleteTVShow)
		r.Get("/{id}/seasons", h.ListSeasons)
		r.Post("/{id}/seasons", h.CreateSeason)
		r.Get("/{id}/seasons/{season}/episodes", h.ListEpisodes)
		r.Post("/{id}/seasons/{season}/episodes", h.CreateEpisode)
	})

	r.Route("/api/v1/genres", func(r chi.Router) {
		r.Get("/", h.ListGenres)
		r.Post("/", h.CreateGenre)
	})

	r.Route("/api/v1/match-queue", func(r chi.Router) {
		r.Get("/", h.ListMatchQueue)
		r.Post("/", h.CreateMatchQueue)
		r.Put("/{id}", h.UpdateMatchQueue)
	})

	r.Get("/api/v1/stats", h.GetStats)

	// Plugin action endpoints (from plugin.json actions)
	r.Post("/search", h.Search)
	r.Post("/match", h.Match)
	r.Post("/match-batch", h.MatchBatch)
	r.Get("/queue", h.GetQueue)
	r.Post("/confirm", h.Confirm)
	r.Post("/refresh", h.Refresh)
	r.Post("/sync", h.Sync)
	r.Get("/status", h.Status)

	return r
}

// serverAddr formats the listen address from config. Extracted so the
// port formatting rule is unit-testable in isolation.
func serverAddr(cfg internal.Config) string {
	return fmt.Sprintf(":%s", cfg.Port)
}

// newServer builds the http.Server for addr wrapping r. Extracted for
// unit-testability of the timeout/address wiring without binding a socket.
func newServer(addr string, r *chi.Mux) *http.Server {
	return &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

func main() {
	cfg := internal.LoadConfig()

	pool, err := internal.NewDB(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	if err := internal.InitSchema(context.Background(), pool); err != nil {
		log.Fatalf("init schema: %v", err)
	}

	h := internal.NewHandlers(pool, cfg)

	r := newRouter(h)

	addr := serverAddr(cfg)
	srv := newServer(addr, r)

	go func() {
		log.Printf("tmdb plugin listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
}
