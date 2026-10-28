package agentapi

// Purpose: the agent carrier of the TEAM coordinator (P7-NODE-20, Epic D5/D6/D7): a TLS 1.3 mTLS HTTP
//          server that accepts node certificates only and serves the runner-protocol routes
//          POST /v1/agent/hello, GET /v1/agent/leases, POST /v1/agent/stream, GET /v1/agent/control and
//          POST /v1/agent/renew (certificate renewal, D-1372).
// Inputs:  Config (CA, deny-list Guard, server certificate source, lease engine, sink); Serve(listener).
// Outputs: Server with Cancel, NodesConnected, CloseNode, Offer, Drain, Upgrade for other coordinator
//          units; protocol frames on the wire (internal/protocol, strict decoding, frames <= 1 MiB).
// Constraints: pki.TLSServerConfig(node) supplies VerifyConnection (kind, chain, deny list, node state;
//          session tickets off); pki.Guard.Middleware(node) checks every request and cancels the live
//          requests of a certificate the moment it is denied; CloseNode additionally closes the node's
//          TCP connections, so a revoke ends long-polls and streams within 1 s; ReadHeaderTimeout and
//          IdleTimeout are set (P7-NODE-07 ruling); no goroutine outlives Shutdown.

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// Leases is the part of the lease engine the carrier uses (*lease.Engine satisfies it).
type Leases interface {
	Reconnect(ctx context.Context, leaseID string, epoch int64, status string) error
	Renew(ctx context.Context, leaseID string, epoch int64, ttl time.Duration) error
	Result(ctx context.Context, leaseID string, epoch int64) error
	Fence(ctx context.Context, leaseID string, epoch int64) error
}

// LeaseSource finds work assigned to a node; it is polled while the node long-polls. A nil lease means none.
type LeaseSource interface {
	Next(ctx context.Context, nodeID string) (*protocol.Lease, error)
}

// Config configures a Server. CA, Guard, Cert, Leases and Sink are required.
type Config struct {
	CA    *pki.CA
	Guard *pki.Guard
	// Cert returns the coordinator server certificate (LeafSource.Get renews it before it expires).
	Cert   func() (tls.Certificate, error)
	Leases Leases
	Sink   transport.Sink
	Source LeaseSource
	// Store is optional: it resolves the epoch of a lease the node never reported (Cancel).
	Store *store.Store
	// OnAck, when set, is called with the authenticated node id and the lease id of every agent ack that
	// names a lease (the drain learns a cancel was acknowledged). It must not block.
	OnAck func(nodeID, leaseID string)
	// Admit, when set, vets every lease-scoped call (a frame or a heartbeat entry) before the lease engine sees it
	// (P7-NODE-08: the job token gate). A refusal drops the call and cancels that lease on the agent.
	Admit func(ctx context.Context, c Call) error
	// Blobs, when set, takes the digest and chunk frames of artifact and cache uploads (P7-NODE-08, CACHE delta 5).
	Blobs BlobHandler
	// PinnedAge returns the age recipient pinned for a node; nil trusts what hello presents.
	PinnedAge func(ctx context.Context, nodeID string) (*string, error)

	Versions        []int         // protocol versions spoken (default {1})
	MinAgentVersion string        // default "0.0.0"
	Heartbeat       time.Duration // default 10 s
	CoordinatorTTL  time.Duration // default 45 s; agent_ttl = TTL - 2 x heartbeat
	PollTimeout     time.Duration // longest long-poll (default 25 s)
	SourcePoll      time.Duration // LeaseSource poll period (default 1 s)
	GuardEvery      time.Duration // deny-list refresh period (default 250 ms; the promise is 1 s)
	Now             func() time.Time
	Logf            func(format string, a ...any)
}

func (c *Config) defaults() error {
	if c.CA == nil || c.Guard == nil || c.Cert == nil || c.Leases == nil || c.Sink == nil {
		return errors.New("agentapi: CA, Guard, Cert, Leases and Sink are required")
	}
	if len(c.Versions) == 0 {
		c.Versions = []int{1}
	}
	if c.MinAgentVersion == "" {
		c.MinAgentVersion = "0.0.0"
	}
	if c.Heartbeat <= 0 {
		c.Heartbeat = 10 * time.Second
	}
	if c.CoordinatorTTL <= 0 {
		c.CoordinatorTTL = 45 * time.Second
	}
	if c.PollTimeout <= 0 {
		c.PollTimeout = 25 * time.Second
	}
	if c.SourcePoll <= 0 {
		c.SourcePoll = time.Second
	}
	if c.GuardEvery <= 0 {
		c.GuardEvery = 250 * time.Millisecond
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.Logf == nil {
		c.Logf = func(string, ...any) {}
	}
	return nil
}

// maxHeaderBytes bounds a request's headers: the upstream stream carries one job token per lease the agent runs
// (up to maxTokens of about 450 bytes each).
const maxHeaderBytes = 128 << 10

// agentTTL is how long an agent may go without a heartbeat ack before it fences itself (Epic D6).
func (c *Config) agentTTL() time.Duration { return c.CoordinatorTTL - 2*c.Heartbeat }

// Server is the agent listener.
type Server struct {
	cfg      Config
	tlsc     *tls.Config
	http     *http.Server
	mu       sync.Mutex
	nodes    map[string]*nodeState
	connNode map[net.Conn]string

	paused atomic.Bool // no new lease is handed to an agent (the drain)
	bg     sync.WaitGroup
	cancel context.CancelFunc
	ln     net.Listener
}

// New validates cfg and builds the server and its TLS config (TLS 1.3, node client certificates only).
func New(cfg Config) (*Server, error) {
	if err := cfg.defaults(); err != nil {
		return nil, err
	}
	first, err := cfg.Cert()
	if err != nil {
		return nil, err
	}
	tlsc, err := cfg.CA.TLSServerConfig(pki.KindNode, first, cfg.Guard)
	if err != nil {
		return nil, err
	}
	tlsc.Certificates = nil // the certificate is fetched per handshake so a renewed one is picked up
	tlsc.GetCertificate = func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		c, e := cfg.Cert()
		return &c, e
	}
	s := &Server{cfg: cfg, tlsc: tlsc, nodes: map[string]*nodeState{}, connNode: map[net.Conn]string{}}
	s.http = &http.Server{
		Handler:           s.Handler(),
		TLSConfig:         tlsc,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    maxHeaderBytes,
		ConnState: func(c net.Conn, st http.ConnState) {
			if st == http.StateClosed || st == http.StateHijacked {
				s.dropConn(c)
			}
		},
		ConnContext: func(ctx context.Context, c net.Conn) context.Context { return context.WithValue(ctx, connKey{}, c) },
	}
	return s, nil
}

// TLSConfig is the listener config (tests dial with a client certificate against it).
func (s *Server) TLSConfig() *tls.Config { return s.tlsc }

// Handler serves the five routes behind the per-request guard (node certificates only).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/agent/hello", s.serveHello)
	mux.HandleFunc("GET /v1/agent/leases", s.serveLeases)
	mux.HandleFunc("POST /v1/agent/stream", s.serveStream)
	mux.HandleFunc("GET /v1/agent/control", s.serveControl)
	mux.HandleFunc("POST /v1/agent/renew", s.serveRenew)
	return s.cfg.Guard.Middleware(pki.KindNode)(mux)
}

// Addr is the bound address once Serve runs.
func (s *Server) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return nil
	}
	return s.ln.Addr()
}

// Serve reads the deny list once, keeps it fresh and serves ln until Shutdown. It returns nil after Shutdown.
func (s *Server) Serve(ln net.Listener) error {
	ctx, cancel := context.WithCancel(context.Background())
	if err := s.cfg.Guard.Refresh(ctx); err != nil {
		cancel()
		return err
	}
	s.mu.Lock()
	s.ln, s.cancel = ln, cancel
	s.mu.Unlock()
	s.bg.Add(1)
	go func() { defer s.bg.Done(); s.cfg.Guard.Run(ctx, s.cfg.GuardEvery) }()
	err := s.http.ServeTLS(ln, "", "")
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown closes every node connection, stops the listener and waits for every goroutine of the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	cancel := s.cancel
	ids := make([]string, 0, len(s.nodes))
	for id := range s.nodes {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		s.CloseNode(id)
	}
	err := s.http.Shutdown(ctx)
	if cancel != nil {
		cancel()
	}
	s.bg.Wait()
	return err
}

// StopOffering makes the lease long-poll answer 204 from now on: a draining coordinator hands out no new
// lease, queued or from the source. Leases an agent already holds are untouched.
func (s *Server) StopOffering() { s.paused.Store(true) }
