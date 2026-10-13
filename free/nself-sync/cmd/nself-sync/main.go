package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/nself-org/nself-sync/internal/auth"
	"github.com/nself-org/nself-sync/internal/store"
	"github.com/nself-org/nself-sync/internal/ws"
)

// DefaultPort is the port registered for nself-sync (F10-PORT-REGISTRY); NSELF_SYNC_PORT or PORT override it.
const DefaultPort = "3853"

// ListenAddr resolves the listen address from the environment. The bind host defaults to loopback
// (NSELF_SYNC_BIND); the image sets 0.0.0.0 so the container port is reachable.
func ListenAddr() string {
	port := firstEnv(DefaultPort, "NSELF_SYNC_PORT", "PORT")
	host := "127.0.0.1"
	if v := os.Getenv("NSELF_SYNC_BIND"); v != "" {
		host = v
	}
	return host + ":" + port
}

// firstEnv returns the first non-empty variable among keys, else def.
func firstEnv(def string, keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return def
}

// server holds wired dependencies. Globals are avoided so tests can build
// per-test instances with fakes.
type server struct {
	snap    store.SnapshotStore
	cursors store.CursorStore
	ops     store.OperationStore
	secret  []byte
}

// snapStoreRef is set at boot in main() and read by free-function handlers
// during the gradual migration to method handlers. Tests construct their own
// server{} and call its methods directly — they never read this ref.
var snapStoreRef atomic.Value // store.SnapshotStore

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatalf("DATABASE_URL not set")
	}
	st, err := store.New(ctx, dsn)
	if err != nil {
		log.Fatalf("store init: %v", err)
	}
	defer st.Close()
	snapStoreRef.Store(store.SnapshotStore(st))
	// Own migrations apply once, right after the pool exists; a failure is fatal.
	mustBootMigrations(ctx, st.Pool())

	secret, err := auth.SecretFromEnv()
	if err != nil {
		log.Fatalf("auth secret: %v", err)
	}
	srvDeps := &server{snap: st, cursors: st, ops: st, secret: secret}

	// WebSocket subscribe factory: one dedicated pg conn per WS session.
	// The factory adapter bridges *store.Listener (pgx.Notification) to
	// ws.Listener (transport-neutral Notification).
	subscribeFactory := func(ctx context.Context) (ws.Listener, error) {
		l, err := st.NewListener(ctx)
		if err != nil {
			return nil, err
		}
		return &pgListenerAdapter{l: l}, nil
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Get("/health", handleHealth)

	r.Route("/sync", func(r chi.Router) {
		r.Post("/push", srvDeps.handlePush)
		r.Post("/pull", handlePull)
		r.Post("/ack", srvDeps.handleAck)
		r.Get("/subscribe", ws.Handler(secret, subscribeFactory))
		r.Get("/snapshot", srvDeps.handleSnapshot)
	})

	addr := ListenAddr()
	srv := &http.Server{Addr: addr, Handler: r}
	go func() {
		log.Printf("nself-sync listening on %s", addr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
}

// pgListenerAdapter bridges *store.Listener (which uses pgconn.Notification
// per pgx v5) to the transport-neutral ws.Listener interface.
type pgListenerAdapter struct {
	l *store.Listener
}

func (a *pgListenerAdapter) Listen(ctx context.Context, channel string) error {
	return a.l.Listen(ctx, channel)
}

func (a *pgListenerAdapter) WaitForNotification(ctx context.Context) (*ws.Notification, error) {
	n, err := a.l.WaitForNotification(ctx)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, nil
	}
	return &ws.Notification{Channel: n.Channel, Payload: n.Payload}, nil
}

func (a *pgListenerAdapter) EventsSince(ctx context.Context, userID string, cur store.Cursor, limit int) ([]store.SyncEvent, error) {
	return a.l.EventsSince(ctx, userID, cur, limit)
}

func (a *pgListenerAdapter) Close(ctx context.Context) error {
	return a.l.Close(ctx)
}
