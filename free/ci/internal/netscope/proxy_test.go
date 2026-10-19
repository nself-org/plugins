package netscope

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// fakeResolver answers from a map and counts lookups (a name must be resolved once per request).
type fakeResolver struct {
	mu      sync.Mutex
	answers map[string][]string
	calls   map[string]int
}

func (r *fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.calls == nil {
		r.calls = map[string]int{}
	}
	r.calls[host]++
	var out []netip.Addr
	for _, s := range r.answers[host] {
		out = append(out, netip.MustParseAddr(s))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no such host %s", host)
	}
	return out, nil
}

type pipeConn struct {
	net.Conn
	remote *net.TCPAddr
}

func (c pipeConn) RemoteAddr() net.Addr { return c.remote }

// echoDialer hands out an in-memory upstream that echoes and records the address it was dialled on.
type echoDialer struct {
	mu     sync.Mutex
	dialed []string
}

func (d *echoDialer) DialContext(_ context.Context, _, address string) (net.Conn, error) {
	d.mu.Lock()
	d.dialed = append(d.dialed, address)
	d.mu.Unlock()
	host, _, _ := net.SplitHostPort(address)
	a, client := net.Pipe()
	go func() { _, _ = io.Copy(client, client); _ = client.Close() }()
	return pipeConn{Conn: a, remote: &net.TCPAddr{IP: net.ParseIP(host), Port: 443}}, nil
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.b.Bytes()...)
}

func startProxy(t *testing.T, scope model.NetworkScope, answers map[string][]string, facts Facts, extra []string) (addr string, res *fakeResolver, d *echoDialer, ev *syncBuf) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	res, d, ev = &fakeResolver{answers: answers}, &echoDialer{}, &syncBuf{}
	p := &Proxy{Guard: Guard{Scope: scope, Facts: facts, ExtraHosts: extra, Resolver: res}, Dialer: d, Events: ev}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = p.Serve(ctx, ln); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return ln.Addr().String(), res, d, ev
}

// connect sends a CONNECT and returns the status line and the connection.
func connect(t *testing.T, proxy, target string) (string, net.Conn, *bufio.Reader) {
	t.Helper()
	c, err := net.DialTimeout("tcp", proxy, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	r := bufio.NewReader(c)
	status, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("no CONNECT answer: %v", err)
	}
	return strings.TrimSpace(status), c, r
}

func TestProxyTunnelsAllowedHostToThePinnedAddress(t *testing.T) {
	addr, res, d, ev := startProxy(t, "restricted", map[string][]string{"proxy.golang.org": {"142.250.80.17"}}, Facts{}, nil)
	status, c, r := connect(t, addr, "proxy.golang.org:443")
	if !strings.Contains(status, "200") {
		t.Fatalf("status %q", status)
	}
	for { // skip the rest of the response header
		line, err := r.ReadString('\n')
		if err != nil || strings.TrimSpace(line) == "" {
			break
		}
	}
	fmt.Fprint(c, "ping\n")
	if got, _ := r.ReadString('\n'); got != "ping\n" {
		t.Fatalf("echo %q", got)
	}
	if res.calls["proxy.golang.org"] != 1 {
		t.Errorf("name resolved %d times", res.calls["proxy.golang.org"])
	}
	if len(d.dialed) != 1 || d.dialed[0] != "142.250.80.17:443" {
		t.Errorf("dialled %v, want the pinned address only", d.dialed)
	}
	if len(ev.bytes()) != 0 {
		t.Errorf("events for an allowed request: %s", ev.bytes())
	}
}

// DNS rebinding: an allowlisted name that resolves to a private address is refused, once, with one event.
func TestProxyRefusesNameResolvingToPrivateAddress(t *testing.T) {
	addr, _, d, ev := startProxy(t, "restricted", map[string][]string{"proxy.golang.org": {"10.0.0.1"}}, Facts{}, nil)
	status, _, _ := connect(t, addr, "proxy.golang.org:443")
	if !strings.Contains(status, "403") {
		t.Fatalf("status %q", status)
	}
	if len(d.dialed) != 0 {
		t.Errorf("dialled %v", d.dialed)
	}
	events := ParseEvents(bytes.NewReader(ev.bytes()))
	if len(events) != 1 || events[0].Reason != ReasonSpecial || events[0].Host != "proxy.golang.org" || events[0].Scope != "restricted" {
		t.Fatalf("events %+v", events)
	}
}

func TestProxyRestrictedAllowlistAndLiterals(t *testing.T) {
	addr, _, d, ev := startProxy(t, "restricted", map[string][]string{"example.org": {"93.184.216.34"}, "ci.example.net": {"93.184.216.35"}}, Facts{}, []string{"ci.example.net"})
	for target, want := range map[string]string{"example.org:443": "403", "93.184.216.34:443": "403", "[2606:4700::1111]:443": "403", "ci.example.net:443": "200"} {
		if status, _, _ := connect(t, addr, target); !strings.Contains(status, want) {
			t.Errorf("%s: %q, want %s", target, status, want)
		}
	}
	if len(d.dialed) != 1 {
		t.Errorf("dialled %v", d.dialed)
	}
	reasons := map[Reason]int{}
	for _, e := range ParseEvents(bytes.NewReader(ev.bytes())) {
		reasons[e.Reason]++
	}
	if reasons[ReasonNotAllowed] != 3 {
		t.Errorf("reasons %v", reasons)
	}
}

func TestProxyInternetRefusesPrivateAndExclusion(t *testing.T) {
	facts := Facts{Exclusion: []netip.Prefix{netip.MustParsePrefix("5.75.235.42/32")}, ExclusionHosts: []string{"prod.example.com"},
		OnLink: []netip.Prefix{netip.MustParsePrefix("2a00:1450:4001::/64")}, HostAddrs: []netip.Addr{netip.MustParseAddr("203.0.113.9")}}
	addr, _, d, ev := startProxy(t, "internet", map[string][]string{
		"ok.example.org": {"93.184.216.34"}, "db.example.org": {"10.0.0.5"}, "mixed.example.org": {"93.184.216.34", "192.168.1.1"},
		"deploy.example.org": {"5.75.235.42"}, "prod.example.com": {"93.184.216.40"}, "api.prod.example.com": {"93.184.216.41"},
		"v6lan.example.org": {"2a00:1450:4001::7"}, "map.example.org": {"::ffff:10.0.0.9"}}, facts, nil)
	for target, want := range map[string]string{
		"ok.example.org:443": "200", "db.example.org:443": "403", "mixed.example.org:443": "403", "deploy.example.org:443": "403",
		"prod.example.com:443": "403", "api.prod.example.com:443": "403", "v6lan.example.org:443": "403", "map.example.org:443": "403",
		"10.0.0.1:443": "403", "[::ffff:10.0.0.1]:443": "403", "169.254.169.254:80": "403", "[64:ff9b::a00:1]:443": "403", "[2002:a00:1::1]:443": "403",
		"127.0.0.1:8080": "403", "5.75.235.42:22": "403", "203.0.113.9:443": "403", "93.184.216.99:443": "200"} {
		if status, _, _ := connect(t, addr, target); !strings.Contains(status, want) {
			t.Errorf("%s: %q, want %s", target, status, want)
		}
	}
	if len(d.dialed) != 2 {
		t.Errorf("dialled %v, want only the two public targets", d.dialed)
	}
	if n := len(ParseEvents(bytes.NewReader(ev.bytes()))); n != 15 {
		t.Errorf("%d denial events, want one per refusal (15): %s", n, ev.bytes())
	}
}

func TestProxyAbsoluteHTTPAndOriginForm(t *testing.T) {
	addr, _, _, ev := startProxy(t, "internet", map[string][]string{"db.example.org": {"10.0.0.5"}}, Facts{}, nil)
	for _, c := range []struct{ req, want string }{
		{"GET http://db.example.org/x HTTP/1.1\r\nHost: db.example.org\r\n\r\n", "403"},
		{"GET /x HTTP/1.1\r\nHost: a\r\n\r\n", "400"},
		{"GET http://10.0.0.1/ HTTP/1.1\r\nHost: 10.0.0.1\r\n\r\n", "403"},
		{"GET ftp://example.org/ HTTP/1.1\r\nHost: a\r\n\r\n", "400"},
	} {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		fmt.Fprint(conn, c.req)
		resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
		_ = conn.Close()
		if err != nil || !strings.HasPrefix(resp.Status, c.want) {
			t.Errorf("%q: %v %v, want %s", c.req, resp, err, c.want)
		}
	}
	for _, e := range ParseEvents(bytes.NewReader(ev.bytes())) {
		if strings.Contains(e.Detail+e.Host, "/x") {
			t.Errorf("event records a path: %+v", e)
		}
	}
}

// restricted serves ports 443 and 80 only: an allowlisted host is not a way to another service on that name.
func TestProxyRestrictedPortPolicy(t *testing.T) {
	addr, _, d, ev := startProxy(t, "restricted", map[string][]string{"registry.npmjs.org": {"104.16.1.1"}}, Facts{}, nil)
	for target, want := range map[string]string{"registry.npmjs.org:443": "200", "registry.npmjs.org:80": "200", "registry.npmjs.org:25": "403", "registry.npmjs.org:22": "403"} {
		if status, _, _ := connect(t, addr, target); !strings.Contains(status, want) {
			t.Errorf("%s: %q, want %s", target, status, want)
		}
	}
	if len(d.dialed) != 2 {
		t.Errorf("dialled %v", d.dialed)
	}
	ports := 0
	for _, e := range ParseEvents(bytes.NewReader(ev.bytes())) {
		if e.Reason == ReasonPort {
			ports++
		}
	}
	if ports != 2 {
		t.Errorf("%d port events", ports)
	}
	if !PortAllowed("internet", "25") || PortAllowed("restricted", "25") {
		t.Error("PortAllowed")
	}
}

func TestConnectionTokensListHopByHopHeaders(t *testing.T) {
	h := http.Header{"Connection": {"X-Secret, close", "keep-alive"}}
	if got := strings.Join(connectionTokens(h), ","); got != "X-Secret,close,keep-alive" {
		t.Errorf("%q", got)
	}
}

// A client that closes its sending side right after the request still gets its destination judged (and its
// tunnel when allowed): the lookup does not run under the request context, which the server cancels on EOF.
func TestProxyJudgesRequestsFromClientsThatHalfClose(t *testing.T) {
	addr, res, d, ev := startProxy(t, "restricted", map[string][]string{"proxy.golang.org": {"142.250.80.17"}, "sum.golang.org": {"10.0.0.7"}}, Facts{}, nil)
	for target, want := range map[string]string{"proxy.golang.org:443": "200", "sum.golang.org:443": "403"} {
		c, err := net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		_ = c.SetDeadline(time.Now().Add(10 * time.Second))
		fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
		_ = c.(*net.TCPConn).CloseWrite()
		status, _ := bufio.NewReader(c).ReadString('\n')
		_ = c.Close()
		if !strings.Contains(status, want) {
			t.Errorf("%s: %q, want %s", target, status, want)
		}
	}
	if res.calls["proxy.golang.org"] != 1 || len(d.dialed) != 1 {
		t.Errorf("lookups %v dialled %v", res.calls, d.dialed)
	}
	for _, e := range ParseEvents(bytes.NewReader(ev.bytes())) {
		if e.Reason == ReasonResolve {
			t.Errorf("a cancelled lookup was recorded as a denial: %+v", e)
		}
	}
}
