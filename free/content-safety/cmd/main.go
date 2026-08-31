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

	"github.com/nself-org/nself-content-safety/internal"
)

func main() {
	cfg := internal.LoadConfig()

	ctx := context.Background()
	db, err := internal.NewDB(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer db.Close()

	if err := db.InitSchema(ctx); err != nil {
		log.Fatalf("init schema: %v", err)
	}

	h := &internal.Handlers{DB: db, Cfg: cfg}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)

	if cfg.APIKey != "" {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				key := req.Header.Get("X-API-Key")
				if key == "" {
					key = req.URL.Query().Get("api_key")
				}
				if key != cfg.APIKey {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
					return
				}
				next.ServeHTTP(w, req)
			})
		})
	}

	r.Get("/health", h.HealthHandler)
	r.Get("/ready", h.ReadyHandler)

	r.Route("/api/v1", func(r chi.Router) {
		// Trust-safety: evidence
		r.Post("/evidence", h.EvidenceCreate)
		r.Get("/evidence", h.EvidenceList)

		// Trust-safety: legal holds
		r.Post("/legal-holds", h.LegalHoldCreate)
		r.Get("/legal-holds", h.LegalHoldList)

		// Trust-safety: evidence exports
		r.Post("/evidence/exports", h.EvidenceExportCreate)
		r.Get("/evidence/exports", h.EvidenceExportList)

		// Trust-safety: statistics
		r.Get("/trust-safety/stats", h.TrustSafetyStats)

		// Spam: analyze
		r.Post("/spam/analyze", h.SpamAnalyze)

		// Spam: config
		r.Get("/spam/config", h.SpamConfigGet)
		r.Put("/spam/config", h.SpamConfigUpdate)

		// Spam: rate limits
		r.Get("/spam/rate-limits", h.RateLimitList)
		r.Post("/spam/rate-limits", h.RateLimitCreate)
		r.Delete("/spam/rate-limits/{id}", h.RateLimitDelete)

		// Spam: rules
		r.Get("/spam/rules", h.SpamRuleList)
		r.Post("/spam/rules", h.SpamRuleCreate)
		r.Delete("/spam/rules/{id}", h.SpamRuleDelete)

		// Raid: status
		r.Get("/raid/status", h.RaidStatusGet)
		r.Post("/raid/status", h.RaidEventCreate)
		r.Put("/raid/status", h.RaidStatusUpdate)

		// Raid: lockdown
		r.Get("/raid/lockdown", h.LockdownGet)
		r.Post("/raid/lockdown", h.LockdownCreate)
		r.Delete("/raid/lockdown/{id}", h.LockdownDelete)

		// Abuse: trust score
		r.Get("/abuse/trust", h.AbuseTrustGet)
		r.Post("/abuse/trust", h.AbuseTrustRegister)
		r.Put("/abuse/trust", h.AbuseTrustUpdate)
	})

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("content-safety plugin listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down...")

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Println("stopped")
}
