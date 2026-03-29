package sdk

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
)

// Server wraps chi.Router with graceful shutdown and a /health endpoint.
type Server struct {
	router chi.Router
	port   int
}

// NewServer creates a chi router, registers GET /health, and returns
// a server ready for route registration.
func NewServer(port int) *Server {
	r := chi.NewRouter()

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	return &Server{
		router: r,
		port:   port,
	}
}

// Router returns the underlying chi.Router so callers can use chi
// features directly (e.g. Route, Group, Mount).
func (s *Server) Router() chi.Router {
	return s.router
}

// Handle registers a handler for the given pattern.
func (s *Server) Handle(pattern string, handler http.Handler) {
	s.router.Handle(pattern, handler)
}

// HandleFunc registers a handler function for the given pattern and HTTP method.
func (s *Server) HandleFunc(pattern, method string, fn http.HandlerFunc) {
	s.router.Method(method, pattern, fn)
}

// ListenAndServe starts the HTTP server and blocks until SIGTERM or SIGINT
// is received. It then initiates a graceful shutdown with a 30-second timeout.
func (s *Server) ListenAndServe() error {
	addr := net.JoinHostPort("", strconv.Itoa(s.port))

	srv := &http.Server{
		Addr:    addr,
		Handler: s.router,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("plugin-sdk: listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	select {
	case sig := <-quit:
		log.Printf("plugin-sdk: received %s, shutting down", sig)
	case err := <-errCh:
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return srv.Shutdown(ctx)
}
