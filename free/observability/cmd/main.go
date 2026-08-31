package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/nself-org/nself-observability/internal"
)

// allowedOriginsSet reads a csv env var and returns the normalized
// allowlist. Wildcard is never accepted.
func allowedOriginsSet(envVar string) map[string]bool {
	set := make(map[string]bool)
	for _, o := range strings.Split(os.Getenv(envVar), ",") {
		o = strings.TrimSpace(strings.ToLower(o))
		if o == "" || o == "*" {
			continue
		}
		set[o] = true
	}
	return set
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
	h.StartWatchdog()

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// CORS — per-origin allowlist from OBSERVABILITY_ALLOWED_ORIGINS (csv).
	allowedCORS := allowedOriginsSet("OBSERVABILITY_ALLOWED_ORIGINS")
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Vary", "Origin")
			origin := req.Header.Get("Origin")
			originAllowed := origin != "" && allowedCORS[strings.ToLower(origin)]
			if originAllowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Source-Account-ID")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
			if req.Method == http.MethodOptions {
				if !originAllowed {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, req)
		})
	})

	r.Get("/health", h.Health)
	r.Get("/ready", h.Ready)

	// Services
	r.Route("/api/v1/services", func(r chi.Router) {
		r.Get("/", h.ListServices)
		r.Post("/", h.CreateService)
		r.Get("/{id}", h.GetService)
		r.Put("/{id}", h.UpdateService)
		r.Delete("/{id}", h.DeleteService)
		r.Post("/{id}/check", h.CheckService)
		r.Get("/{id}/history", h.ListServiceHistory)
	})

	// Health summary
	r.Get("/api/v1/health", h.HealthSummary)

	// Watchdog events
	r.Route("/api/v1/watchdog-events", func(r chi.Router) {
		r.Get("/", h.ListWatchdogEvents)
		r.Post("/", h.CreateWatchdogEvent)
	})

	// Stats & incidents
	r.Get("/api/v1/stats", h.GetStats)
	r.Get("/api/v1/incidents", h.ListIncidents)

	// Dynamic config schema endpoints (OpenClaw pattern).
	r.Route("/monitoring", func(r chi.Router) {
		r.Get("/config/schema", internal.HandleMonitoringConfigSchema)
		r.Get("/ai/instructions", internal.HandleMonitoringAIInstructions)
		r.Get("/config", h.HandleMonitoringGetConfig)
		r.Put("/config", h.HandleMonitoringPutConfig)
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("observability plugin listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down observability plugin...")
	h.StopWatchdog()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Println("observability plugin stopped")
}
