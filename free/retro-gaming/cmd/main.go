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
	"github.com/nself-org/nself-retro-gaming/internal"
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

	h := internal.NewHandlers(pool)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/health", h.Health)
	r.Get("/ready", h.Ready)

	// ROMs
	r.Route("/api/v1/roms", func(r chi.Router) {
		r.Get("/", h.ListROMs)
		r.Post("/", h.CreateROM)
		r.Get("/{id}", h.GetROM)
		r.Put("/{id}", h.UpdateROM)
		r.Delete("/{id}", h.DeleteROM)
	})

	// Save States
	r.Route("/api/v1/save-states", func(r chi.Router) {
		r.Get("/", h.ListSaveStates)
		r.Post("/", h.CreateSaveState)
		r.Get("/{id}", h.GetSaveState)
		r.Put("/{id}", h.UpdateSaveState)
		r.Delete("/{id}", h.DeleteSaveState)
	})

	// Play Sessions
	r.Route("/api/v1/play-sessions", func(r chi.Router) {
		r.Get("/", h.ListPlaySessions)
		r.Post("/", h.CreatePlaySession)
		r.Get("/{id}", h.GetPlaySession)
		r.Put("/{id}", h.UpdatePlaySession)
		r.Delete("/{id}", h.DeletePlaySession)
	})

	// Emulator Cores
	r.Route("/api/v1/emulator-cores", func(r chi.Router) {
		r.Get("/", h.ListEmulatorCores)
		r.Post("/", h.CreateEmulatorCore)
		r.Get("/{id}", h.GetEmulatorCore)
		r.Put("/{id}", h.UpdateEmulatorCore)
		r.Delete("/{id}", h.DeleteEmulatorCore)
	})

	// Controller Configs
	r.Route("/api/v1/controller-configs", func(r chi.Router) {
		r.Get("/", h.ListControllerConfigs)
		r.Post("/", h.CreateControllerConfig)
		r.Get("/{id}", h.GetControllerConfig)
		r.Put("/{id}", h.UpdateControllerConfig)
		r.Delete("/{id}", h.DeleteControllerConfig)
	})

	// Core Installations
	r.Route("/api/v1/core-installations", func(r chi.Router) {
		r.Get("/", h.ListCoreInstallations)
		r.Post("/", h.CreateCoreInstallation)
		r.Get("/{id}", h.GetCoreInstallation)
		r.Delete("/{id}", h.DeleteCoreInstallation)
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("retro-gaming plugin listening on :%s", cfg.Port)
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
