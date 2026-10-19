package main

// Purpose: the agent mTLS listener of `ci serve` (P7-NODE-20, Epic D5/D7): coordinator.RegisterListener
//          ("agent", --agent-addr, ":3864", ...) whose start serves internal/coordinator/agentapi on a
//          TLS 1.3 listener that accepts node certificates only, and registers the revoke hook that closes a
//          revoked node's live long-polls and streams (lifecycle.RegisterSessionCloser).
// Inputs:  the address from --agent-addr; the coordinator's store and data dir (useAgentStore; the
//          fallback opens the default CI home); an optional public URL whose host becomes the SANs of the
//          server certificate; the lease sink and lease source other units install (agentSink, agentLeases).
// Outputs: a running agentapi.Server until stop; stop shuts it down and waits for its goroutines.
// Constraints: no client certificate, a user-kind or coordinator-kind certificate, or a revoked node
//          certificate never reaches a handler (pki.TLSServerConfig + pki.RequestGuard); the server
//          certificate is renewed at 2/3 of its life; the default sink REFUSES logs and results until
//          the agent transport (P7-NODE-08) installs a real one, so nothing is silently dropped; a
//          revoke run by the CLI in another process reaches this one through the deny list within
//          250 ms, a revoke run in this process closes the connections at once.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

const agentDefaultAddr = ":3864"

func init() {
	if err := coordinator.RegisterListener("agent", "agent-addr", agentDefaultAddr, startAgentListener); err != nil {
		panic(err)
	}
	lifecycle.RegisterSessionCloser(closeAgentSessions)
}

var (
	// agentStoreSource yields the coordinator's store and data dir; useAgentStore replaces it.
	agentStoreSource = openDefaultAgentStore
	// agentSink and agentLeases are installed by the units that own the transport and the scheduler.
	agentSink   = func(*store.Store) transport.Sink { return refusingSink{} }
	agentLeases func(*store.Store) agentapi.LeaseSource
	// agentWire, when set, adjusts the agentapi config before the server is built and returns a function that
	// receives the built server (P7-NODE-08: the pull transport installs its sink, lease source, token gate
	// and cancel path; it replaces agentSink and agentLeases).
	agentWire func(cfg *agentapi.Config, d agentDeps) (func(*agentapi.Server), error)
	// agentOnAck, when set, hears every agent ack that names a lease (nodeID, leaseID); the drain sets it.
	agentOnAck func(nodeID, leaseID string)
	agentLogf  = func(format string, a ...any) { fmt.Fprintf(os.Stderr, "nself-ci-serve: "+format+"\n", a...) }
	// activeAgent is the running server, for the session closer.
	activeAgent atomic.Pointer[agentapi.Server]
)

// agentDeps is what the pull transport needs to wire itself to the running listener.
type agentDeps struct {
	CA      *pki.CA
	Store   *store.Store
	DataDir string
}

// agentStore is what the listener needs from the running coordinator.
type agentStore struct {
	Store     *store.Store
	DataDir   string
	PublicURL string
	closeFn   func()
}

// useAgentStore hands the coordinator's own store, data dir and --public-url to the agent listener;
// serveV15 calls it right after it opens the store.
func useAgentStore(st *store.Store, dataDir, publicURL string) {
	agentStoreSource = func() (agentStore, error) {
		return agentStore{Store: st, DataDir: dataDir, PublicURL: publicURL, closeFn: func() {}}, nil
	}
}

func openDefaultAgentStore() (agentStore, error) {
	dir := records.StoreDir("")
	st, err := cirun.OpenStore(filepath.Join(dir, "state.db"))
	if err != nil {
		return agentStore{}, err
	}
	return agentStore{Store: st, DataDir: dir, closeFn: func() { _ = st.Close() }}, nil
}

// closeAgentSessions is the lifecycle session closer: a revoke in this process ends the node's streams now.
func closeAgentSessions(_ context.Context, nodeID string) error {
	if s := activeAgent.Load(); s != nil {
		s.CloseNode(nodeID)
	}
	return nil
}

// refusingSink refuses every agent output until a real sink is installed: an agent that cannot deliver
// keeps its frames and retries, which is safer than a coordinator that acknowledges and drops them.
type refusingSink struct{}

var errNoAgentSink = errors.New("no agent output sink is installed")

func (refusingSink) Log(context.Context, protocol.Log) error                 { return errNoAgentSink }
func (refusingSink) ArtifactRef(context.Context, protocol.ArtifactRef) error { return errNoAgentSink }
func (refusingSink) Result(context.Context, protocol.Result) error           { return errNoAgentSink }
func (refusingSink) CacheRef(context.Context, protocol.CacheRef) error       { return errNoAgentSink }
func (refusingSink) Blob(context.Context, transport.BlobHeader) (transport.BlobWriter, error) {
	return nil, errNoAgentSink
}

// agentSANs are the names the server certificate answers to: the host of --public-url when given, the
// host of the bind address when it names one, and always the loopback names.
func agentSANs(publicURL, bindAddr string) (dns []string, ips []net.IP, err error) {
	seenDNS, seenIP := map[string]bool{}, map[string]bool{}
	add := func(host string) {
		if host == "" {
			return
		}
		if ip := net.ParseIP(host); ip != nil {
			if !ip.IsUnspecified() && !seenIP[ip.String()] {
				seenIP[ip.String()] = true
				ips = append(ips, ip)
			}
			return
		}
		if !seenDNS[host] {
			seenDNS[host] = true
			dns = append(dns, host)
		}
	}
	if publicURL != "" {
		u, perr := url.Parse(publicURL)
		if perr != nil || u.Hostname() == "" {
			return nil, nil, fmt.Errorf("--public-url %q has no host", publicURL)
		}
		add(u.Hostname())
	}
	if host, _, serr := net.SplitHostPort(bindAddr); serr == nil {
		add(host)
	}
	add("localhost")
	add("127.0.0.1")
	add("::1")
	return dns, ips, nil
}

func startAgentListener(ctx context.Context, addr string, _ *coordinator.Coordinator) (func(context.Context) error, error) {
	src, err := agentStoreSource()
	if err != nil {
		return nil, fmt.Errorf("agent listener: store: %w", err)
	}
	fail := func(e error) (func(context.Context) error, error) { src.closeFn(); return nil, e }
	ca, err := pki.New(ctx, pki.Options{Store: src.Store, DataDir: src.DataDir})
	if err != nil {
		return fail(fmt.Errorf("agent listener: PKI: %w", err))
	}
	dns, ips, err := agentSANs(src.PublicURL, addr)
	if err != nil {
		return fail(err)
	}
	leaf := &agentapi.LeafSource{CA: ca, ID: "local", DNS: dns, IPs: ips}
	cfg := agentapi.Config{
		CA: ca, Guard: pki.NewGuard(src.Store), Cert: leaf.Get, Store: src.Store,
		Leases: lease.New(src.Store, registry.New(src.Store), lease.Config{}), Sink: agentSink(src.Store), Logf: agentLogf,
		OnAck: func(nodeID, leaseID string) {
			if fn := agentOnAck; fn != nil {
				fn(nodeID, leaseID)
			}
		},
	}
	if agentLeases != nil {
		cfg.Source = agentLeases(src.Store)
	}
	var attach func(*agentapi.Server)
	if agentWire != nil {
		if attach, err = agentWire(&cfg, agentDeps{CA: ca, Store: src.Store, DataDir: src.DataDir}); err != nil {
			return fail(fmt.Errorf("agent listener: transport: %w", err))
		}
	}
	srv, err := agentapi.New(cfg)
	if err != nil {
		return fail(err)
	}
	if attach != nil {
		attach(srv)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fail(fmt.Errorf("agent listener: cannot listen on %s: %w", addr, err))
	}
	done := make(chan error, 1)
	go func() {
		e := srv.Serve(ln)
		if e != nil {
			agentLogf("agent listener stopped: %v", e)
		}
		done <- e
	}()
	activeAgent.Store(srv)
	agentLogf("agent listener (mTLS, node certificates only) on %s", ln.Addr())
	var once sync.Once
	var stopErr error
	return func(sctx context.Context) error {
		once.Do(func() {
			activeAgent.CompareAndSwap(srv, nil)
			stopErr = srv.Shutdown(sctx)
			select {
			case e := <-done:
				stopErr = errors.Join(stopErr, e)
			case <-time.After(10 * time.Second):
				stopErr = errors.Join(stopErr, errors.New("agent listener did not stop in 10 s"))
			}
			src.closeFn()
		})
		return stopErr
	}, nil
}
