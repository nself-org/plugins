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
	"github.com/nself-org/nself-sports/internal"
	"github.com/nself-org/nself-sports/internal/cron"
	"github.com/nself-org/nself-sports/internal/provider"
	"github.com/nself-org/nself-sports/internal/provider/apifootball"
	"github.com/nself-org/nself-sports/internal/provider/thesportsdb"
)

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

	// Build provider.
	var prov provider.Provider
	switch cfg.Provider {
	case "thesportsdb":
		c, err := thesportsdb.New()
		if err != nil {
			log.Printf("warning: thesportsdb provider unavailable: %v", err)
		} else {
			prov = c
		}
	default: // "apifootball"
		c, err := apifootball.New()
		if err != nil {
			log.Printf("warning: apifootball provider unavailable: %v", err)
		} else {
			prov = c
		}
	}

	h := internal.NewHandlersWithProvider(pool, prov)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// Health
	r.Get("/health", h.Health)
	r.Get("/ready", h.Ready)

	// Leagues
	r.Route("/api/v1/leagues", func(r chi.Router) {
		r.Get("/", h.ListLeagues)
		r.Post("/", h.CreateLeague)
		r.Get("/{id}", h.GetLeague)
		r.Put("/{id}", h.UpdateLeague)
		r.Delete("/{id}", h.DeleteLeague)
	})

	// Teams
	r.Route("/api/v1/teams", func(r chi.Router) {
		r.Get("/", h.ListTeams)
		r.Post("/", h.CreateTeam)
		r.Get("/{id}", h.GetTeam)
		r.Put("/{id}", h.UpdateTeam)
		r.Delete("/{id}", h.DeleteTeam)
	})

	// Events
	r.Route("/api/v1/events", func(r chi.Router) {
		r.Get("/", h.ListEvents)
		r.Post("/", h.CreateEvent)
		r.Get("/{id}", h.GetEvent)
		r.Put("/{id}", h.UpdateEvent)
		r.Delete("/{id}", h.DeleteEvent)
	})

	// Standings
	r.Route("/api/v1/standings", func(r chi.Router) {
		r.Get("/", h.ListStandings)
		r.Post("/", h.UpsertStanding)
	})

	// Favorites
	r.Route("/api/v1/favorites", func(r chi.Router) {
		r.Get("/", h.ListFavorites)
		r.Post("/", h.CreateFavorite)
		r.Delete("/{id}", h.DeleteFavorite)
	})

	// Provider Syncs
	r.Route("/api/v1/syncs", func(r chi.Router) {
		r.Get("/", h.ListSyncs)
		r.Post("/", h.CreateSync)
	})

	// Webhook Events
	r.Route("/api/v1/webhook-events", func(r chi.Router) {
		r.Get("/", h.ListWebhookEvents)
		r.Post("/", h.CreateWebhookEvent)
	})

	// Schedule Cache
	r.Route("/api/v1/cache", func(r chi.Router) {
		r.Get("/", h.ListCache)
		r.Post("/", h.UpsertCache)
	})

	// Sync trigger routes
	r.Post("/sports/sync/leagues", h.SyncLeagues)
	r.Post("/sports/sync/teams", h.SyncTeams)
	r.Post("/sports/sync/events", h.SyncEvents)
	r.Post("/sports/sync/live", h.SyncLiveScores)

	// Live scores + EPG
	r.Get("/sports/live", h.LiveScores)
	r.Get("/sports/epg", h.EPG)

	// Cron worker (optional).
	if cfg.SyncCronEnabled && prov != nil {
		w := cron.New(pool, prov, os.Getenv("SPORTS_SOURCE_ACCOUNT_ID"))
		cronCtx, cronCancel := context.WithCancel(context.Background())
		go w.Start(cronCtx)
		defer cronCancel()
		log.Println("sports cron worker enabled")
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("sports plugin listening on :%s", cfg.Port)
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
