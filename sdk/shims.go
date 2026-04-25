package sdk

// shims.go exposes convenience top-level functions that delegate to the
// sub-packages (db, middleware, server). This allows plugin main packages
// to import just "github.com/nself-org/plugin-sdk" and call sdk.ConnectDB,
// sdk.NewServer, sdk.Recovery, sdk.Logger, sdk.CORS, sdk.RequestID without
// managing multiple sub-package imports.

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	sdkdb "github.com/nself-org/plugin-sdk/db"
	sdkmw "github.com/nself-org/plugin-sdk/middleware"
)

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
func (s *Server) ListenAndServe() error {
	addr := fmt.Sprintf(":%d", s.port)
	srv := &http.Server{
		Addr:    addr,
		Handler: s.router,
	}
	log.Printf("[sdk] listening on %s", addr)
	return srv.ListenAndServe()
}

// NewServer returns a Server with a new chi router bound to port.
func NewServer(port int) *Server {
	return &Server{router: chi.NewRouter(), port: port}
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
