package discovery

// Purpose: LAN candidate discovery (Epic D12, R-DX-22, R-SEC-13). The Guard is the single entry point:
//          Browse (only `nself ci nodes discover` calls it) and Advertise (only `nself-ci-agent join
//          --advertise` calls it, for one pairing window). The mDNS library opens its own sockets, so
//          every socket open sits behind Allowed() here and nowhere else.
// Inputs:  a Net (the mDNS library in production, an in-memory responder in tests), the environment, the
//          user config.
// Outputs: Candidates (state discovered, never authorized) and an Advertisement bound to its window.
// Constraints: no socket is opened unless a command asked for it and Allowed() is nil; a record found on
//          the LAN is a hint and never an identity: nothing here pairs, authorizes or registers a node.

import (
	"context"
	"fmt"
	"io"
	"net/netip"
	"sync"
	"time"
)

const (
	// ServiceType is the DNS-SD service the fabric browses and advertises.
	ServiceType = "_nself-ci._tcp"
	// Domain is the mDNS domain.
	Domain = "local."
	// MaxWindow is the longest advertisement: the pairing code lifetime (internal/pairing.OfferTTL).
	MaxWindow = 10 * time.Minute
	// SSHPort is the port a candidate record names: the push carrier's port, advisory only.
	SSHPort = 22
	// MinTimeout and MaxTimeout bound one browse.
	MinTimeout = 500 * time.Millisecond
	// RawCap is the most raw records one browse keeps; reaching it marks the result flooded.
	RawCap     = 4 * MaxCandidates
	MaxTimeout = 15 * time.Second
)

// Entry is one raw record from the network: untrusted input.
type Entry struct {
	Name, Host string
	Addrs      []netip.Addr
	Port       int
	TXT        []string
}

// Service is what an advertisement publishes.
type Service struct {
	Instance, Type, Domain, HostName string
	Port                             int
	IPs                              []netip.Addr
	TXT                              []string
}

// Net is the socket layer. Only the Guard calls it.
type Net interface {
	// Browse queries for the service type for at most timeout and returns what answered.
	Browse(ctx context.Context, serviceType string, timeout time.Duration) ([]Entry, error)
	// Advertise starts answering for svc and returns the function that stops it.
	Advertise(svc Service) (stop func(), err error)
	// LocalAddrs lists this host's addresses worth publishing.
	LocalAddrs() []netip.Addr
}

// Guard gates every use of the network. The zero value uses the mDNS library, the real environment and
// the default config path.
type Guard struct {
	Home   string                               // CI home holding config.yaml ("" means $NSELF_CI_HOME or ~/.nself/ci)
	Getenv func(string) string                  // nil means os.Getenv
	Net    Net                                  // nil means the mDNS library
	Rand   io.Reader                            // nil means crypto/rand
	Now    func() time.Time                     // nil means time.Now
	After  func(time.Duration) <-chan time.Time // nil means time.After
}

func (g *Guard) net() Net {
	if g.Net != nil {
		return g.Net
	}
	return systemNet{}
}

func (g *Guard) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}

// Browse lists the candidates that answer within timeout. It returns an E667 error, before any socket
// is opened, when discovery is off.
func (g *Guard) Browse(ctx context.Context, timeout time.Duration) (Result, error) {
	if err := g.Allowed(); err != nil {
		return Result{}, err
	}
	if timeout < MinTimeout || timeout > MaxTimeout {
		return Result{}, fmt.Errorf("discovery: --timeout must be between %s and %s", MinTimeout, MaxTimeout)
	}
	entries, err := g.net().Browse(ctx, ServiceType, timeout)
	if err != nil {
		return Result{}, fmt.Errorf("discovery: browse failed: %w", err)
	}
	r := candidates(entries)
	r.Flooded = len(entries) >= RawCap
	return r, nil
}

// Advertisement is a running advertisement bound to one pairing window.
type Advertisement struct {
	WindowID string    // the TXT record, printed by join so the admin can match the candidate
	Instance string    // random, for tests and logs
	HostName string    // random SRV target and A-record name
	Expires  time.Time // the window end

	once sync.Once
	stop func()
	done chan struct{}
}

// Done is closed when the advertisement has stopped: window end, Stop, or the context ending.
func (a *Advertisement) Done() <-chan struct{} { return a.done }

// Stop ends the advertisement now. It is safe to call more than once.
func (a *Advertisement) Stop() {
	a.once.Do(func() {
		a.stop()
		close(a.done)
	})
}

// Announce prints the line the admin needs to match this node in `nodes discover`.
func (a *Advertisement) Announce(w io.Writer) {
	_, _ = fmt.Fprintf(w, "LAN discovery id: %s (visible to `nself ci nodes discover` until %s)\n", a.WindowID, a.Expires.Local().Format(time.TimeOnly))
}

// Advertise publishes one record for window (at most MaxWindow) and stops it when the window ends or ctx
// does. It returns an E667 error, before any socket is opened, when discovery is off. A nil ips means
// this host's own addresses.
func (g *Guard) Advertise(ctx context.Context, window time.Duration, port int, ips []netip.Addr) (*Advertisement, error) {
	if err := g.Allowed(); err != nil {
		return nil, err
	}
	if window <= 0 || window > MaxWindow {
		return nil, fmt.Errorf("discovery: the advertisement window must be between 1ns and %s", MaxWindow)
	}
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("discovery: invalid advertisement port %d", port)
	}
	n := g.net()
	if len(ips) == 0 {
		ips = n.LocalAddrs()
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("discovery: this host has no address to advertise")
	}
	id, err := newIdentity(g.Rand)
	if err != nil {
		return nil, err
	}
	stop, err := n.Advertise(Service{Instance: id.Instance, Type: ServiceType, Domain: Domain, HostName: id.Host, Port: port, IPs: ips, TXT: []string{id.ID}})
	if err != nil {
		return nil, fmt.Errorf("discovery: advertise failed: %w", err)
	}
	a := &Advertisement{WindowID: id.ID, Instance: id.Instance, HostName: id.Host, Expires: g.now().Add(window), stop: stop, done: make(chan struct{})}
	after := g.After
	if after == nil {
		after = time.After
	}
	end := after(window)
	go func() {
		select {
		case <-end:
		case <-ctx.Done():
		case <-a.done:
		}
		a.Stop()
	}()
	return a, nil
}
