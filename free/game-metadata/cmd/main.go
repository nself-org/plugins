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
	"github.com/nself-org/nself-game-metadata/internal"
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
		log.Fatalf("init schema: %v", err)
	}

	h := internal.NewHandlers(pool)
	r := newRouter(h)
	srv := newServer(cfg, r)

	go func() {
		log.Printf("game-metadata plugin listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	if err := shutdown(srv, 30*time.Second); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
}

// newRouter wires up all HTTP routes and middleware for the game-metadata
// plugin. Extracted from main() so it can be exercised directly in tests
// (route registration + middleware wiring) without booting a real server or
// connecting to a database.
func newRouter(h *internal.Handlers) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/health", h.Health)
	r.Get("/ready", h.Ready)

	// Platforms
	r.Route("/api/v1/platforms", func(r chi.Router) {
		r.Get("/", h.ListPlatforms)
		r.Post("/", h.CreatePlatform)
		r.Get("/{id}", h.GetPlatform)
		r.Delete("/{id}", h.DeletePlatform)
	})

	// Genres
	r.Route("/api/v1/genres", func(r chi.Router) {
		r.Get("/", h.ListGenres)
		r.Post("/", h.CreateGenre)
		r.Get("/{id}", h.GetGenre)
		r.Delete("/{id}", h.DeleteGenre)
	})

	// Games
	r.Route("/api/v1/games", func(r chi.Router) {
		r.Get("/", h.ListGames)
		r.Post("/", h.CreateGame)
		r.Get("/search", h.SearchGames)
		r.Get("/lookup", h.LookupByHash)
		r.Get("/{id}", h.GetGame)
		r.Delete("/{id}", h.DeleteGame)
		r.Route("/{game_id}/metadata", func(r chi.Router) {
			r.Get("/", h.GetGameMetadata)
			r.Put("/", h.UpsertGameMetadata)
		})
		r.Route("/{game_id}/artwork", func(r chi.Router) {
			r.Get("/", h.ListArtwork)
			r.Post("/", h.CreateArtwork)
		})
	})

	// Artwork delete by ID
	r.Delete("/api/v1/artwork/{id}", h.DeleteArtwork)

	// Stats
	r.Get("/api/v1/stats", h.GetStats)

	return r
}

// newServer builds the *http.Server for the game-metadata plugin. Extracted
// from main() so the address + timeout wiring is directly testable without
// booting a real listener.
func newServer(cfg internal.Config, h http.Handler) *http.Server {
	return &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      h,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

// shutdowner is the subset of *http.Server used by shutdown(), extracted so
// tests can substitute a stub instead of binding a real listener.
type shutdowner interface {
	Shutdown(ctx context.Context) error
}

// shutdown gracefully stops srv, bounded by timeout. Extracted from main()
// so the shutdown-context wiring and error propagation are directly
// testable via a stub server.
func shutdown(srv shutdowner, timeout time.Duration) error {
	shutCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return srv.Shutdown(shutCtx)
}
