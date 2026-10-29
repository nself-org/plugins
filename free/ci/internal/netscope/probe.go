package netscope

// Purpose: the probe that proves which scopes a runner really enforces (capability fact
// network.enforced_scopes, Epic D4/D11). SCHED derives Declaration.NoLANRoute from it; a runner never declares it.
// Inputs:  the runtime binary the container profile probed and the planner that builds scopes.
// Outputs: the scopes proven now: none whenever a runtime exists, restricted and internet only when a real
//          job network with its sidecar could not reach the host, a host listener or a raw internet address.
// Constraints: the probe builds the same network and sidecar a job gets (a per-attempt id, removed after), then
//          runs one container on it that dials the host's own addresses, the metadata address and a raw public
//          address; anything it reaches, or any connection the host listener accepts, withholds both scopes.
//          A probe that cannot run claims nothing. Results are cached briefly.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

const (
	probeTTL     = 10 * time.Minute
	probeFailTTL = time.Minute
	// maxProbeChunk bounds the targets of one probe run (the runtime argv carries at most 64).
	maxProbeChunk = 40
)

// rawTargets are addresses a job must never reach directly, whatever the scope: the cloud metadata address
// and a public address (raw egress below lan is impossible because the internal network has no route).
var rawTargets = []string{"169.254.169.254:80", "1.1.1.1:443"}

// Prober proves scopes. The zero value is usable.
type Prober struct {
	Planner *Planner
	Now     func() time.Time
	// Listen opens the host fixture the probe tries to reach (default: any address, ephemeral port).
	Listen func() (net.Listener, error)

	mu     sync.Mutex
	at     time.Time
	ttl    time.Duration
	scopes []model.NetworkScope
	binary string
}

func (pr *Prober) now() time.Time {
	if pr.Now != nil {
		return pr.Now()
	}
	return time.Now()
}

// Invalidate drops the cached result, so the next call probes again (the runtime changed or went away).
func (pr *Prober) Invalidate() {
	pr.mu.Lock()
	pr.at, pr.scopes, pr.binary = time.Time{}, nil, ""
	pr.mu.Unlock()
}

// EnforcedScopes returns the scopes proven for the runtime binary. An empty binary (no container runtime)
// proves nothing.
func (pr *Prober) EnforcedScopes(ctx context.Context, binary string) []model.NetworkScope {
	if binary == "" {
		return nil
	}
	pr.mu.Lock()
	defer pr.mu.Unlock()
	if pr.binary == binary && !pr.at.IsZero() && pr.now().Sub(pr.at) < pr.ttl {
		return append([]model.NetworkScope(nil), pr.scopes...)
	}
	scopes := []model.NetworkScope{"none"}
	ttl := probeFailTTL
	if pr.proveProxied(ctx, binary) {
		scopes = append(scopes, "restricted", "internet")
		ttl = probeTTL
	}
	pr.binary, pr.at, pr.ttl, pr.scopes = binary, pr.now(), ttl, scopes
	return append([]model.NetworkScope(nil), scopes...)
}

func (pr *Prober) proveProxied(ctx context.Context, binary string) bool {
	pl := pr.Planner
	if pl == nil {
		pl = &Planner{}
	}
	id, err := probeID()
	if err != nil {
		return false
	}
	// the probe builds the internet scope: the same network and sidecar as restricted, but an IP literal is judged
	// by the deny table instead of the name allowlist, so the metadata check below exercises the table. Its
	// deliberate refusals are not recorded as events.
	quiet := *pl
	quiet.Events = func(string, Event) {}
	pl = &quiet
	plan, err := pl.PlanAttempt(ctx, binary, isolation.Attempt{ID: id, CoordinatorID: "probe"}, "internet")
	if plan.Release != nil {
		defer plan.Release(ctx)
	}
	if err != nil {
		return false
	}
	listen := pr.Listen
	if listen == nil {
		listen = func() (net.Listener, error) { return net.Listen("tcp", ":0") }
	}
	ln, err := listen()
	if err != nil {
		return false
	}
	defer ln.Close()
	accepted := make(chan struct{}, 64)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
			select {
			case accepted <- struct{}{}:
			default: // already counted; never block on a scanner hitting the port
			}
		}
	}()
	targets, err := pl.probeTargets(ln)
	control := proxyAddress(plan.Env)
	if err != nil || control == "" {
		return false
	}
	bin, removeBinary, err := pl.proxyBinary()
	if err != nil {
		return false
	}
	defer removeBinary()
	image := pl.Image
	if image == "" {
		image = ProxyImage
	}
	// every host address is dialled (in chunks: the runtime argv is bounded), each chunk with the positive
	// control: "nothing was reached" only means something if the probe could connect at all, so the proxy must
	// be the one target that answered. The first run also checks the two other ways out: the proxy must refuse
	// a tunnel to the metadata address, and a DNS lookup in the job network must fail.
	for i := 0; i < len(targets) || i == 0; i += maxProbeChunk {
		chunk := targets[min(i, len(targets)):min(i+maxProbeChunk, len(targets))]
		out, err := pl.rt().ProbeRun(ctx, binary, id, image, bin, control, append(append([]string(nil), chunk...), control))
		if err != nil {
			return false
		}
		got := Reached(out)
		if len(got) != 1 || got[0] != control || !probeCompleted(out) {
			return false
		}
		if i == 0 && (!proxyRefused(out) || !dnsFailed(out)) {
			return false
		}
	}
	time.Sleep(200 * time.Millisecond) // let the fixture's accept loop count a connection that just completed
	return len(accepted) == 0
}

// proxyAddress extracts host:port of the proxy from the job's proxy variables.
func proxyAddress(env []string) string {
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "HTTPS_PROXY=http://"); ok {
			return v
		}
	}
	return ""
}

// probeTargets lists what the probe dials: every host address on the fixture's port plus the raw targets.
func (p *Planner) probeTargets(ln net.Listener) ([]string, error) {
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		return nil, err
	}
	host := p.Host
	if host == nil {
		host = Discover
	}
	f, err := host()
	if err != nil {
		return nil, err
	}
	var targets []string
	for _, a := range f.HostAddrs {
		if a = Unmap(a); a.IsValid() && !a.IsLoopback() && !a.IsLinkLocalUnicast() {
			targets = append(targets, net.JoinHostPort(a.String(), port))
		}
	}
	return append(targets, rawTargets...), nil
}

// The probe container prints one "reached <target>" line per connect that succeeded and a final "done" line.
const (
	reachedPrefix = "reached "
	doneLine      = "done"
)

// Reached parses the probe output: the targets it reached.
func Reached(out []byte) []string {
	var got []string
	for _, line := range strings.Split(string(out), "\n") {
		if t, ok := strings.CutPrefix(strings.TrimSpace(line), reachedPrefix); ok {
			got = append(got, t)
		}
	}
	return got
}

// probeCompleted reports whether the probe printed its final line: a run cut short proves nothing.
func probeCompleted(out []byte) bool {
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == doneLine {
			return true
		}
	}
	return false
}

// RunProbeMode is the container side: dial every target and print what connected; ask the proxy for a tunnel
// to the metadata address and print the status it answered with; try one DNS lookup and print whether it
// resolved; then "done".
func RunProbeMode(ctx context.Context, proxy string, targets []string, w interface{ Write([]byte) (int, error) }) {
	for _, t := range ProbeDial(ctx, targets, 3*time.Second) {
		_, _ = w.Write([]byte(reachedPrefix + t + "\n"))
	}
	if proxy != "" {
		_, _ = w.Write([]byte("proxy " + tunnelStatus(ctx, proxy, "169.254.169.254:80") + "\n"))
	}
	lctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if addrs, err := net.DefaultResolver.LookupHost(lctx, "example.com"); err == nil && len(addrs) > 0 {
		_, _ = w.Write([]byte("dns resolved\n"))
	} else {
		_, _ = w.Write([]byte("dns failed\n"))
	}
	_, _ = w.Write([]byte(doneLine + "\n"))
}

// tunnelStatus sends a CONNECT for target to the proxy and returns the status code it answers ("error" when
// it does not answer).
func tunnelStatus(ctx context.Context, proxy, target string) string {
	d := net.Dialer{Timeout: 5 * time.Second}
	c, err := d.DialContext(ctx, "tcp", proxy)
	if err != nil {
		return "error"
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(8 * time.Second))
	_, _ = c.Write([]byte("CONNECT " + target + " HTTP/1.1\r\nHost: " + target + "\r\n\r\n"))
	buf := make([]byte, 64)
	n, _ := c.Read(buf)
	if f := strings.Fields(string(buf[:n])); len(f) >= 2 && f[0] == "HTTP/1.1" {
		return f[1]
	}
	return "error"
}

// proxyRefused reports that the probe's tunnel request to the metadata address was answered 403.
func proxyRefused(out []byte) bool { return hasLine(out, "proxy 403") }

// dnsFailed reports that the probe's DNS lookup failed.
func dnsFailed(out []byte) bool { return hasLine(out, "dns failed") }

func hasLine(out []byte, want string) bool {
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

func probeID() (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "probe-" + hex.EncodeToString(b[:]), nil
}
