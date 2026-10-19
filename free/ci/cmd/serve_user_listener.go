package main

// Purpose: the user API and local-socket listeners of the V15 `ci serve` (P7-NODE-19, Epic D14):
// `--api-addr` (default 127.0.0.1:3865, TLS 1.3, user certificates or scoped tokens) and `--api-socket`
// ($NSELF_CI_HOME/serve.sock, mode 0600, the operator). Both register through coordinator.RegisterListener
// and flags, so serve_v15.go is unchanged.
// Inputs:  the serve flags --data-dir, --api-addr, --api-socket, --api-public-url; the coordinator's store.
// Outputs: HTTPS on the API address; a unix socket and the serve.lock flock while the serve runs.
// Constraints: the server certificate comes from the coordinator PKI (SANs: localhost, loopback, the bind
//          host and --api-public-url) and is renewed in place before it expires; an empty --api-addr or
//          --api-socket turns that listener off; the socket is the operator's and takes the exclusive
//          serve lock, so a second serve on the same home refuses to start; the repo allowlist of the
//          coordinator applies to submits (an empty allowlist allows none).

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/coordclient"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// userAPIFlags is the serve flag set the listeners read --data-dir and --api-public-url from.
var userAPIFlags struct {
	sync.Mutex
	fs *flag.FlagSet
}

func init() {
	coordinator.RegisterFlag(func(fs *flag.FlagSet) {
		fs.String("api-public-url", "", "URL users reach the user API at (its host goes into the server certificate)")
		userAPIFlags.Lock()
		userAPIFlags.fs = fs
		userAPIFlags.Unlock()
	})
	for _, l := range []struct {
		name, flag, def string
		start           coordinator.ListenerStart
	}{
		{"user-api", "api-addr", "127.0.0.1:3865", startUserAPI},
		{"local-socket", "api-socket", "auto", startLocalSocket},
	} {
		if err := coordinator.RegisterListener(l.name, l.flag, l.def, l.start); err != nil {
			panic("nself-ci: " + err.Error())
		}
	}
}

// serveFlag reads a parsed serve flag.
func serveFlag(name string) string {
	userAPIFlags.Lock()
	defer userAPIFlags.Unlock()
	if userAPIFlags.fs == nil {
		return ""
	}
	if f := userAPIFlags.fs.Lookup(name); f != nil {
		return f.Value.String()
	}
	return ""
}

// explicitFlag reports whether the operator passed the serve flag.
func explicitFlag(name string) bool {
	userAPIFlags.Lock()
	defer userAPIFlags.Unlock()
	set := false
	if userAPIFlags.fs != nil {
		userAPIFlags.fs.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	}
	return set
}

// optional turns a failure of a default-on listener into a log line, so an existing deployment whose port
// or path is taken keeps serving webhooks; a listener the operator asked for by flag fails the start.
func optional(c *coordinator.Coordinator, flagName string, err error) (func(context.Context) error, error) {
	if explicitFlag(flagName) {
		return nil, err
	}
	c.Logf("--%s listener is off: %v (pass --%s to choose another or to make this an error)", flagName, err, flagName)
	return noop, nil
}

// serveHome is the CI home of the running serve.
func serveHome() string { return records.StoreDir(serveFlag("data-dir")) }

// userRoles builds the role service over the coordinator's store.
func userRoles(c *coordinator.Coordinator) *roles.Service {
	audit.SetScanner(cirun.Redactions)
	svc := roles.New(c.Store(), &audit.Log{St: c.Store(), Scrub: cirun.Redactions})
	svc.Self = operatorPrincipal()
	roles.Use(svc)
	return svc
}

// apiSANs are the names the server certificate answers to.
func apiSANs(addr, publicURL string) (dns []string, ips []net.IP, err error) {
	dns, ips = []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	if publicURL != "" {
		d, i, err := pki.SANsFromURL(publicURL)
		return append(dns, d...), append(ips, i...), err
	}
	if host, _, err := net.SplitHostPort(addr); err == nil && host != "" {
		if ip := net.ParseIP(host); ip != nil {
			if !ip.IsUnspecified() {
				ips = append(ips, ip)
			}
		} else {
			dns = append(dns, host)
		}
	}
	return dns, ips, nil
}

// leafSource issues the coordinator server certificate and renews it when a third of its life is left.
func leafSource(ca *pki.CA, id string, dns []string, ips []net.IP) func() (tls.Certificate, error) {
	var mu sync.Mutex
	var cur tls.Certificate
	var born, until time.Time
	return func() (tls.Certificate, error) {
		mu.Lock()
		defer mu.Unlock()
		if now := ca.Now(); until.IsZero() || until.Sub(now) < until.Sub(born)/3 {
			c, err := ca.IssueServer(id, dns, ips, 0)
			if err != nil {
				return tls.Certificate{}, err
			}
			cur, born, until = c, now, now.Add(pki.DefaultTTL)
		}
		return cur, nil
	}
}

// stopping maps a failure caused by the serve shutting down to "not started", any other to an error.
func stopping(ctx context.Context, err error) (func(context.Context) error, error) {
	if ctx.Err() != nil {
		return noop, nil
	}
	return nil, err
}

// noop is the stop function of a listener that was not started.
func noop(context.Context) error { return nil }

func startUserAPI(ctx context.Context, addr string, c *coordinator.Coordinator) (func(context.Context) error, error) {
	if ctx.Err() != nil {
		return noop, nil // the serve is already stopping
	}
	home := serveHome()
	svc := userRoles(c)
	ca, err := pki.New(ctx, pki.Options{Store: c.Store(), DataDir: home})
	if err != nil {
		return stopping(ctx, err)
	}
	if p := serveFlag("api-public-url"); p != "" {
		if u, e := url.Parse(p); e != nil || u.Scheme != "https" {
			return nil, errors.New("--api-public-url must be an https URL")
		}
	}
	dns, ips, err := apiSANs(addr, serveFlag("api-public-url"))
	if err != nil {
		return nil, err
	}
	guard := pki.NewGuard(c.Store())
	if err = guard.Refresh(ctx); err != nil {
		return stopping(ctx, err)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return optional(c, "api-addr", err)
	}
	leaf := leafSource(ca, "coord-"+shortHost(), dns, ips)
	if _, err = leaf(); err != nil { // fail at start, not at the first handshake
		_ = ln.Close()
		return nil, err
	}
	api := clientapi.New(clientapi.Config{Roles: svc, Allow: c.RepoAllowed, Logf: c.Logf,
		Backend: &clientapi.StoreBackend{Store: c.Store(), Home: home, Enqueue: c.Enqueue}})
	gctx, cancel := context.WithCancel(ctx)
	go guard.Run(gctx, 2*time.Second)
	c.Logf("user API listening on %s (TLS 1.3, user certificates or scoped tokens)", ln.Addr())
	return serveOn(gctx, tls.NewListener(ln, clientapi.TLSConfig(ca, leaf, guard)), api.Handler(), cancel, nil), nil
}

// serveOn runs h on ln in the background. stop ends it and then runs after.
func serveOn(ctx context.Context, ln net.Listener, h http.Handler, cancel context.CancelFunc, after func()) func(context.Context) error {
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = clientapi.Serve(ctx, ln, h)
	}()
	return func(sctx context.Context) error {
		cancel()
		select {
		case <-done:
		case <-sctx.Done():
		}
		if after != nil {
			after()
		}
		return nil
	}
}

func shortHost() string {
	h, _ := os.Hostname()
	h = strings.ToLower(strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return '-'
	}, h))
	if h = strings.Trim(h, "-"); h == "" || len(h) > 40 {
		return "host"
	}
	return h
}

// startLocalSocket binds serve.sock and takes the serve lock. The caller on this socket is the operator:
// the socket is 0600 inside the owner's CI home, which is the same trust as the data dir itself (Epic D1).
func startLocalSocket(ctx context.Context, addr string, c *coordinator.Coordinator) (func(context.Context) error, error) {
	if ctx.Err() != nil {
		return noop, nil // the serve is already stopping
	}
	home := serveHome()
	path := addr
	if path == "auto" {
		path = coordclient.SocketPath(home)
	}
	release, err := coordclient.AcquireServeLock(home) // a second serve on this home is a mistake: refuse it
	if err != nil {
		return nil, err
	}
	ln, err := clientapi.ListenUnix(path)
	if err != nil {
		release()
		return optional(c, "api-socket", err)
	}
	api := clientapi.New(clientapi.Config{Roles: userRoles(c), Local: true, Operator: adminActor(), Logf: c.Logf,
		Backend: &clientapi.StoreBackend{Store: c.Store(), Home: home, Enqueue: c.Enqueue}})
	gctx, cancel := context.WithCancel(ctx)
	c.Logf("local socket %s (mode 0600) held by pid %d", path, os.Getpid())
	return serveOn(gctx, ln, api.Handler(), cancel, func() { _ = os.Remove(path); release() }), nil
}
