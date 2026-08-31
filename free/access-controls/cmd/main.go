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

	"github.com/nself-org/nself-access-controls/internal"
)

func main() {
	cfg := internal.LoadConfig()

	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	db, err := internal.NewDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.InitSchema(ctx); err != nil {
		log.Fatalf("schema initialization failed: %v", err)
	}

	h := internal.NewHandlers(db, cfg)
	r := newRouter(h, cfg)

	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("access-controls plugin listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	log.Println("stopped")
}

// newRouter wires up all HTTP routes and middleware for the access-controls
// plugin. Extracted from main() so it can be exercised directly in tests
// (route registration + middleware wiring) without booting a real server or
// connecting to a database.
func newRouter(h *internal.Handlers, cfg internal.Config) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(corsMiddleware)

	// Health endpoints (no auth).
	r.Get("/health", h.Health)
	r.Get("/ready", h.Ready)

	// API routes.
	r.Route("/api/v1", func(r chi.Router) {
		if cfg.APIKey != "" {
			r.Use(apiKeyMiddleware(cfg.APIKey))
		}

		// Roles.
		r.Get("/roles", h.ListRoles)
		r.Post("/roles", h.CreateRole)
		r.Get("/roles/{id}", h.GetRole)
		r.Put("/roles/{id}", h.UpdateRole)
		r.Delete("/roles/{id}", h.DeleteRole)

		// Role permissions.
		r.Get("/roles/{id}/permissions", h.ListRolePermissions)
		r.Post("/roles/{id}/permissions", h.AssignPermissionToRole)
		r.Delete("/roles/{id}/permissions/{permission_id}", h.RemovePermissionFromRole)

		// Permissions.
		r.Get("/permissions", h.ListPermissions)
		r.Post("/permissions", h.CreatePermission)
		r.Get("/permissions/{id}", h.GetPermission)
		r.Put("/permissions/{id}", h.UpdatePermission)
		r.Delete("/permissions/{id}", h.DeletePermission)

		// User roles.
		r.Get("/users/{user_id}/roles", h.GetUserRoles)
		r.Post("/users/{user_id}/roles", h.AssignRoleToUser)
		r.Delete("/users/{user_id}/roles/{role_id}", h.RemoveRoleFromUser)

		// Access check.
		r.Post("/check", h.CheckAccess)

		// Policies.
		r.Get("/policies", h.ListPolicies)
		r.Post("/policies", h.CreatePolicy)
		r.Get("/policies/{id}", h.GetPolicy)
		r.Put("/policies/{id}", h.UpdatePolicy)
		r.Delete("/policies/{id}", h.DeletePolicy)
	})

	return r
}

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

func corsMiddleware(next http.Handler) http.Handler {
	allowed := allowedOriginsSet("ACCESS_CONTROLS_ALLOWED_ORIGINS")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Vary", "Origin")
		origin := r.Header.Get("Origin")
		originAllowed := origin != "" && allowed[strings.ToLower(origin)]
		if originAllowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Source-Account-Id")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			if !originAllowed {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func apiKeyMiddleware(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := r.Header.Get("X-API-Key")
			if got == "" {
				got = r.Header.Get("Authorization")
				if len(got) > 7 && got[:7] == "Bearer " {
					got = got[7:]
				}
			}
			if got != key {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
