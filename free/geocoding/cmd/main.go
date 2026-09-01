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

	"github.com/nself-org/nself-geocoding/internal"
	"github.com/nself-org/nself-geocoding/internal/provider"
	"github.com/nself-org/nself-geocoding/internal/ratelimit"
)

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

	db, err := internal.NewDB(cfg)
	if err != nil {
		log.Fatalf("db init: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.InitSchema(ctx); err != nil {
		log.Fatalf("schema init: %v", err)
	}

	// Build provider from config.
	var prov provider.Provider
	switch strings.ToLower(strings.SplitN(cfg.Providers, ",", 2)[0]) {
	case "mapbox":
		if cfg.MapboxToken == "" {
			log.Fatal("GEOCODING_MAPBOX_TOKEN is required when provider=mapbox")
		}
		prov = provider.NewMapbox(cfg.MapboxToken)
		log.Printf("geocoding provider: mapbox")
	default: // nominatim (default)
		prov = provider.NewNominatim(cfg.NominatimURL, cfg.NominatimUA)
		log.Printf("geocoding provider: nominatim (%s)", cfg.NominatimURL)
	}

	// Rate limiter: GEOCODING_RATE_LIMIT_RPM (default 60 req/min per IP).
	limiter := ratelimit.New(cfg.RateLimitMax)

	h := &internal.Handlers{DB: db, Cfg: cfg, Prov: prov, Limiter: limiter}

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// CORS — per-origin allowlist from GEOCODING_ALLOWED_ORIGINS (csv).
	allowedCORS := allowedOriginsSet("GEOCODING_ALLOWED_ORIGINS")
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Vary", "Origin")
			origin := req.Header.Get("Origin")
			originAllowed := origin != "" && allowedCORS[strings.ToLower(origin)]
			if originAllowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Source-Account-Id, X-API-Key")
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

	// API key auth middleware
	if cfg.APIKey != "" {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				// Skip health probes
				if req.URL.Path == "/health" || req.URL.Path == "/ready" || req.URL.Path == "/live" {
					next.ServeHTTP(w, req)
					return
				}
				key := req.Header.Get("X-API-Key")
				if key == "" {
					key = req.URL.Query().Get("api_key")
				}
				if key != cfg.APIKey {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnauthorized)
					fmt.Fprint(w, `{"error":"unauthorized"}`)
					return
				}
				next.ServeHTTP(w, req)
			})
		})
	}

	// Health
	r.Get("/health", h.Health)
	r.Get("/ready", h.Ready)
	r.Get("/live", h.Health)

	// Geocoding — legacy paths (v1 API)
	r.Post("/api/v1/geocode", h.Geocode)
	r.Post("/api/v1/reverse", h.Reverse)

	// Geocoding — spec paths (SP-08.N01)
	r.Post("/geocode/forward", h.ForwardGeocode)
	r.Post("/geocode/reverse", h.ReverseGeocode)
	r.Post("/geocode/batch-forward", h.BatchForward)
	r.Post("/geocode/batch-reverse", h.BatchReverse)
	r.Get("/geocode/autocomplete", h.Autocomplete)
	r.Get("/geocode/tz", h.Timezone)

	// Geofences
	r.Get("/api/v1/geofences", h.ListGeofences)
	r.Post("/api/v1/geofences", h.CreateGeofence)
	r.Post("/api/v1/geofences/check", h.CheckGeofence)
	r.Get("/api/v1/geofences/{id}", h.GetGeofence)
	r.Put("/api/v1/geofences/{id}", h.UpdateGeofence)
	r.Delete("/api/v1/geofences/{id}", h.DeleteGeofence)

	// Places
	r.Get("/api/v1/places", h.ListPlaces)
	r.Post("/api/v1/places", h.CreatePlace)
	r.Get("/api/v1/places/{id}", h.GetPlace)
	r.Delete("/api/v1/places/{id}", h.DeletePlace)

	// Cache
	r.Get("/api/v1/cache", h.ListCache)
	r.Delete("/api/v1/cache/{id}", h.DeleteCacheEntry)

	// Stats & quota
	r.Get("/api/v1/stats", h.Stats)
	r.Get("/api/v1/quota", h.GetQuota)

	addr := cfg.Host + ":" + cfg.Port
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("geocoding plugin listening on http://%s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down...")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
	log.Println("stopped")
}
