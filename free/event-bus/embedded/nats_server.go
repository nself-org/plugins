// Package embedded manages the lifecycle of an in-process NATS server.
// When NATS_EMBEDDED=true (the default), the event-bus plugin starts its own
// NATS JetStream instance inside the same Go process — no external broker needed.
package embedded

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats-server/v2/server"
)

// Server wraps an embedded NATS server.
type Server struct {
	ns *server.Server
}

// Start launches the embedded NATS server on the given client port.
// JetStream is enabled with the provided storeDir for persistent storage.
func Start(clientPort int, storeDir string) (*Server, error) {
	opts := &server.Options{
		ServerName:   "nself-event-bus",
		Port:         clientPort,
		JetStream:    true,
		StoreDir:     storeDir,
		NoLog:        false,
		NoSigs:       true,
		MaxPayload:   server.MAX_PAYLOAD_SIZE,
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		return nil, fmt.Errorf("embedded nats: new server: %w", err)
	}

	ns.ConfigureLogger()
	go ns.Start()

	if !ns.ReadyForConnections(10 * time.Second) {
		ns.Shutdown()
		return nil, fmt.Errorf("embedded nats: server did not become ready within 10 s")
	}

	slog.Info("embedded NATS started", "port", clientPort, "store_dir", storeDir)
	return &Server{ns: ns}, nil
}

// ClientURL returns the nats:// URL callers should use to connect.
func (s *Server) ClientURL() string {
	return s.ns.ClientURL()
}

// Shutdown gracefully stops the embedded server.
func (s *Server) Shutdown() {
	if s.ns != nil {
		s.ns.Shutdown()
		s.ns.WaitForShutdown()
		slog.Info("embedded NATS stopped")
	}
}
