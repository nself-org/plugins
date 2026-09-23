package sdk

// shims.go exposes convenience top-level functions that delegate to the
// sub-packages (db, middleware, server). This allows plugin main packages
// to import just "github.com/nself-org/plugin-sdk" and call sdk.ConnectDB,
// sdk.NewServer, sdk.Recovery, sdk.Logger, sdk.CORS, sdk.RequestID without
// managing multiple sub-package imports.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	sdkdb "github.com/nself-org/plugin-sdk/db"
	sdkmw "github.com/nself-org/plugin-sdk/middleware"
)

// Config holds common plugin configuration loaded from environment variables.
type Config struct {
	DatabaseURL string
	Port        int
}

// LoadConfig reads DATABASE_URL and PORT from the environment.
// Port defaults to 3000 when PORT is unset or unparseable.
func LoadConfig() *Config {
	port := 3000
	if p := os.Getenv("PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			port = n
		}
	}
	return &Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Port:        port,
	}
}

// ConnectDB opens a PostgreSQL connection pool using default nSelf settings.
// dsn must be a postgres:// or postgresql:// URL.
func ConnectDB(dsn string) (*pgxpool.Pool, error) {
	return sdkdb.Open(context.Background(), sdkdb.PoolConfig{DSN: dsn})
}

// Server is a thin wrapper around a chi router and port so callers can use the
// sdk.NewServer(port) pattern and get a .Router() + .ListenAndServe() API.
type Server struct {
	router chi.Router
	port   int
}

// Router returns the chi.Router for registering routes and middleware.
func (s *Server) Router() chi.Router {
	return s.router
}

// ListenAndServe starts the HTTP server on the configured port.
//
// It registers the default liveness routes (via ensureHealthRoutes) just
// before the listener starts, rather than NewServer registering them at
// construction time — see ensureHealthRoutes for why.
func (s *Server) ListenAndServe() error {
	ensureHealthRoutes(s.router)
	addr := fmt.Sprintf(":%d", s.port)
	srv := &http.Server{
		Addr:    addr,
		Handler: s.router,
	}
	log.Printf("[sdk] listening on %s", addr)
	return srv.ListenAndServe()
}

// NewServer returns a Server with a new, empty chi router bound to port.
// The default liveness endpoints (see ensureHealthRoutes) are NOT registered
// here — they are deferred to ListenAndServe. Route registration is
// intentionally NOT done in this constructor: chi panics with "all
// middlewares must be defined before routes on a mux" if any route exists
// on the router before Router().Use(...) is called, and the standard plugin
// pattern is:
//
//	srv := sdk.NewServer(port)
//	r := srv.Router()
//	r.Use(sdk.Recovery, sdk.Logger, ...)
//	internal.RegisterRoutes(r, ...)
//	srv.ListenAndServe()
//
// A NewServer that pre-registered /healthz, /health, /readyz would make
// every plugin's r.Use(...) call above panic, because those three routes
// would already exist on the mux by the time Use runs. This was PR #104's
// regression: notify and cron (built on exactly this pattern) panicked at
// startup in the nSelf golden-path E2E (2026-09-21) with that exact chi
// panic, because the unit test added in #104 never called Use() after
// NewServer and so never exercised the ordering.
func NewServer(port int) *Server {
	r := chi.NewRouter()
	return &Server{router: r, port: port}
}

// ensureHealthRoutes registers the default liveness endpoints every plugin
// container's docker-compose healthcheck depends on:
//
//	GET /healthz - liveness (always 200 once the server is up)
//	GET /health  - liveness alias (the nSelf CLI's docker-compose healthcheck
//	               convention, internal/compose/custom_services.go, probes
//	               /health; every plugin.json documents /health as the
//	               health_endpoint)
//	GET /readyz  - readiness; this thin shim has no dependency-check hook, so
//	               it mirrors liveness. Plugins needing real readiness checks
//	               (DB ping, upstream API) should override /readyz via
//	               Router() before calling ListenAndServe.
//
// It is called from ListenAndServe rather than from NewServer — see
// NewServer's doc comment for why registering routes at construction time
// breaks every plugin's Router().Use(...) call with a chi panic.
//
// Each path/method pair is registered ONLY if the router has no handler for
// it yet (checked via chi's Match, which walks the routing tree without
// executing a handler). This means a plugin that registers its own GET
// /health before calling ListenAndServe keeps that handler — this default
// never overrides an explicit plugin route, matching the pre-#104-regression
// contract ("callers remain free to register their own /health*/readyz on
// Router()").
//
// HEAD is registered alongside GET on the same terms. The nSelf CLI's
// docker-compose healthcheck template probes with `wget --spider`, which
// sends HEAD, not GET (internal/compose/custom_services.go). Before this,
// ensureHealthRoutes registered GET only, so every plugin on the default
// chi router had no route for HEAD /health at all; chi's default 405
// handler answered "405 Method Not Allowed" and the container never went
// healthy (nself golden-path E2E, free/cron, 2026-09-22). HEAD is checked
// independently of GET: a plugin that registers its own GET /health but no
// HEAD /health still gets the default HEAD handler, because GET and HEAD
// are different entries in chi's routing tree and a GET handler is never
// invoked for a HEAD request.
func ensureHealthRoutes(r chi.Router) {
	for _, path := range []string{"/healthz", "/health", "/readyz"} {
		if !r.Match(chi.NewRouteContext(), http.MethodGet, path) {
			r.Get(path, handleLiveness)
		}
		if !r.Match(chi.NewRouteContext(), http.MethodHead, path) {
			r.Head(path, handleLivenessHead)
		}
	}
}

// handleLiveness is the default liveness responder shared by /healthz,
// /health, and /readyz on a shim-created Server. See ensureHealthRoutes's
// doc comment for why /health must exist and why /readyz has no dependency
// check here.
func handleLiveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

// handleLivenessHead answers HEAD requests for /healthz, /health, and
// /readyz with a bare 200 and no body. It deliberately does not delegate to
// handleLiveness and re-encode the JSON body: per RFC 9110 a HEAD response
// must not have a body, and net/http's real Server would silently discard
// any body written on a HEAD request anyway (chunkWriter.Write "eats"
// writes for HEAD) — but httptest.ResponseRecorder, used in this package's
// tests, does not emulate that discard, so writing a body here would pass
// against a live server yet fail the unit test. Keeping this handler
// write-free makes both environments agree.
func handleLivenessHead(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}

// Recovery is an HTTP middleware that recovers from panics and returns 500.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[sdk] panic recovered: %v", rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Logger is an HTTP middleware that logs each request method, path, and status.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		log.Printf("[sdk] %s %s %d", r.Method, r.URL.Path, rw.status)
	})
}

// statusRecorder wraps ResponseWriter to capture the status code for logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// CORS is an HTTP middleware that adds permissive CORS headers.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequestID is an HTTP middleware that injects a unique request ID header.
// Delegates to the sdk/middleware package.
func RequestID(next http.Handler) http.Handler {
	return sdkmw.RequestID(next)
}

// sourceAccountHeaders are the accepted spellings of the multi-app isolation
// key header. Hasura forwards this under different casings depending on how the
// Action is wired; accepting all four prevents silent tenant merging when only
// one spelling is checked. Canonical pattern: free/e2ee/internal/auth.go.
var sourceAccountHeaders = []string{
	"X-Source-Account-ID",
	"X-Source-Account-Id",
	"X-Hasura-Source-Account-Id",
	"X-Source-Account",
}

// SourceAccountID extracts the multi-app isolation account ID from an HTTP
// request. It accepts all four canonical header spellings (X-Source-Account-ID,
// X-Source-Account-Id, X-Hasura-Source-Account-Id, X-Source-Account) and returns
// "primary" only when none are present — matching the default value used in all
// np_* table columns. Checking a single spelling silently merged tenants whose
// gateway forwarded a different casing (multi-tenant isolation bug, P4 E0).
//
// Purpose: DRY helper used by all plugins that enforce source_account_id isolation.
// Inputs:  r — the incoming HTTP request.
// Outputs: account ID string, never empty.
// Constraints: Must not be called on requests that bypass the Hasura proxy.
// SPORT: F08-SERVICE-INVENTORY — multi-app isolation pattern.
func SourceAccountID(r *http.Request) string {
	for _, name := range sourceAccountHeaders {
		if v := strings.TrimSpace(r.Header.Get(name)); v != "" {
			return v
		}
	}
	return "primary"
}
