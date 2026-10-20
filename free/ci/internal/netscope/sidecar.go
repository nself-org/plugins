package netscope

// Purpose: the entrypoint of the proxy sidecar: the code `nself-ci netproxy` runs inside the minimal image.
// Inputs:  the encoded Config in the environment, the container's own interfaces, stdout for events.
// Outputs: a listener on the sidecar's address inside the job network and the served proxy; ProbeDial for the
//          scope probe.
// Constraints: the proxy listens only on its job-network address, never on the bridge side, so a container on
// the default bridge cannot use it; the sidecar's own prefixes and addresses join the deny Facts; a config
// that does not parse stops the sidecar before it listens.

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"
)

// RunSidecar serves until ctx ends. cfg is the encoded Config; events receives one line per refusal.
func RunSidecar(ctx context.Context, cfg string, events interface{ Write([]byte) (int, error) }) error {
	c, facts, err := DecodeConfig(cfg)
	if err != nil {
		return err
	}
	own, err := Discover()
	if err != nil {
		return err
	}
	facts = Merge(facts, own)
	job, _ := netip.ParsePrefix(c.JobPrefix)
	listen, ok := addrIn(job, own.HostAddrs)
	if !ok {
		return scopeFailed("the sidecar has no address in the job network %s", job)
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(listen.String(), portString()))
	if err != nil {
		return scopeFailed("listen on %s: %v", listen, err)
	}
	defer ln.Close()
	_, _ = events.Write(ReadyLine())
	p := &Proxy{
		Guard:  Guard{Scope: c.Scope, Facts: facts, ExtraHosts: c.Hosts, Resolver: net.DefaultResolver},
		Dialer: &net.Dialer{Timeout: dialTimeout},
		Events: events,
	}
	return p.Serve(ctx, ln)
}

func portString() string { return strconv.Itoa(ProxyPort) }

func addrIn(p netip.Prefix, addrs []netip.Addr) (netip.Addr, bool) {
	for _, a := range addrs {
		if p.Contains(a) {
			return a, true
		}
	}
	return netip.Addr{}, false
}

// ProbeDial tries a TCP connect to each address, all at once, and returns the ones that connected in input
// order. It is the probe's only network action: the probe container runs it on the job network against
// addresses that must not be reachable.
func ProbeDial(ctx context.Context, addrs []string, timeout time.Duration) []string {
	ok := make([]bool, len(addrs))
	var wg sync.WaitGroup
	d := net.Dialer{Timeout: timeout}
	for i, a := range addrs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c, err := d.DialContext(ctx, "tcp", a); err == nil {
				_ = c.Close()
				ok[i] = true
			}
		}()
	}
	wg.Wait()
	var reached []string
	for i, a := range addrs {
		if ok[i] {
			reached = append(reached, a)
		}
	}
	return reached
}
