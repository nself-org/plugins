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
	"github.com/nself-org/nself-documents/internal"
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

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// Health
	r.Get("/health", h.Health)
	r.Get("/ready", h.Ready)

	// Documents
	r.Route("/api/v1/documents", func(r chi.Router) {
		r.Get("/", h.ListDocuments)
		r.Post("/", h.CreateDocument)
		r.Get("/{id}", h.GetDocument)
		r.Put("/{id}", h.UpdateDocument)
		r.Delete("/{id}", h.DeleteDocument)
		r.Post("/{id}/publish", h.PublishDocument)
		r.Post("/{id}/archive", h.ArchiveDocument)
		r.Get("/{id}/versions", h.ListVersions)
		r.Post("/{id}/versions", h.CreateVersion)
		r.Get("/{id}/shares", h.ListShares)
		r.Post("/{id}/shares", h.CreateShare)
	})

	// Shares — standalone delete
	r.Delete("/api/v1/shares/{id}", h.DeleteShare)

	// Templates
	r.Route("/api/v1/templates", func(r chi.Router) {
		r.Get("/", h.ListTemplates)
		r.Post("/", h.CreateTemplate)
		r.Get("/{id}", h.GetTemplate)
		r.Put("/{id}", h.UpdateTemplate)
		r.Delete("/{id}", h.DeleteTemplate)
	})

	// Stats
	r.Get("/api/v1/stats", h.Stats)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("documents plugin listening on :%s", cfg.Port)
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
	log.Println("documents plugin stopped")
}
