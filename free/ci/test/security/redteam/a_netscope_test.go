//go:build security

package redteam

// Purpose: red team attacks on the no-LAN scopes and the egress proxy (TM-04, TM-05, TM-10 network side).
//   - A01..: a job asks the proxy for LAN, loopback, link-local, CGNAT, ULA, metadata and host addresses in
//     every spelling (decimal, octal, hex, short form, IPv4-mapped IPv6, trailing dots, zones);
//   - DNS rebinding, split answers, a dial that lands on another address, host header and port tricks,
//     allowlist suffix and userinfo confusion.
// Inputs:  netscope.Guard and netscope.Proxy over in-process fakes (resolver, dialer) on a loopback listener.
// Outputs: pass (control held) or fail (attack worked).
// Constraints: no real network; the dialer records every address it is asked to dial and never connects out.

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/netscope"
)

// aFakeResolver answers from a table; answers may change per call to model DNS rebinding.
type aFakeResolver struct {
	mu      sync.Mutex
	answers map[string][][]netip.Addr
	calls   map[string]int
}

func (f *aFakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	seq, ok := f.answers[host]
	if !ok {
		return nil, errors.New("no such host")
	}
	i := f.calls[host]
	f.calls[host]++
	if i >= len(seq) {
		i = len(seq) - 1
	}
	return seq[i], nil
}

// aRecDialer records dial targets; it hands back one end of a pipe whose far end answers an HTTP request.
type aRecDialer struct {
	mu     sync.Mutex
	dialed []string
	seen   chan *http.Request
}

func (d *aRecDialer) DialContext(_ context.Context, _, address string) (net.Conn, error) {
	d.mu.Lock()
	d.dialed = append(d.dialed, address)
	d.mu.Unlock()
	c, s := net.Pipe()
	go func() {
		defer s.Close()
		req, err := http.ReadRequest(bufio.NewReader(s))
		if err != nil {
			return
		}
		if d.seen != nil {
			d.seen <- req
		}
		_, _ = s.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"))
	}()
	return c, nil
}

func (d *aRecDialer) count() int { d.mu.Lock(); defer d.mu.Unlock(); return len(d.dialed) }

func aLanFacts() netscope.Facts {
	return netscope.Facts{
		OnLink:    []netip.Prefix{netip.MustParsePrefix("203.0.114.0/24"), netip.MustParsePrefix("2a00:1450:4001::/64")},
		HostAddrs: []netip.Addr{netip.MustParseAddr("203.0.114.7")},
		Exclusion: []netip.Prefix{netip.MustParsePrefix("198.51.101.5/32")},
	}
}

// A01: every sensitive address is refused in every spelling, under both proxy scopes, when it is a literal
// or the answer of a name.
func TestA01_SensitiveAddressesInEverySpelling(t *testing.T) {
	sensitive := []string{
		"127.0.0.1", "127.255.255.254", "0.0.0.0", "10.0.0.1", "172.16.0.1", "172.31.255.255", "192.168.1.1",
		"169.254.169.254", "169.254.170.2", "100.64.0.1", "100.100.100.200", "192.0.0.192", "168.63.129.16",
		"::1", "::", "fd00:ec2::254", "fc00::1", "fe80::1", "fe80::1%eth0", "::ffff:10.0.0.1", "::ffff:169.254.169.254",
		"::ffff:a00:1", "::10.0.0.1", "64:ff9b::a00:1", "2002:a00:1::1", "2001:0:4136:e378:8000:63bf:3fff:fdd2",
		"ff02::1", "224.0.0.1", "255.255.255.255", "203.0.114.9", "198.51.101.5", "203.0.114.7", "2a00:1450:4001::5",
	}
	for _, scope := range []string{"restricted", "internet"} {
		for _, lit := range sensitive {
			g := netscope.Guard{Scope: aModelScope(scope), Facts: aLanFacts(), ExtraHosts: []string{lit}, Resolver: &aFakeResolver{}}
			if scope == "internet" {
				if _, v := g.Pin(context.Background(), lit); v.Allowed {
					t.Errorf("%s: literal %q pinned", scope, lit)
				}
			}
			a := netip.MustParseAddr(lit)
			if v := netscope.Allowed(aModelScope(scope), a, aLanFacts()); v.Allowed {
				t.Errorf("%s: Allowed(%q) = allowed", scope, lit)
			}
			// a name that resolves to it
			res := &aFakeResolver{answers: map[string][][]netip.Addr{"evil.example": {{a}}}}
			g = netscope.Guard{Scope: aModelScope(scope), Facts: aLanFacts(), ExtraHosts: []string{"evil.example"}, Resolver: res}
			if _, v := g.Pin(context.Background(), "evil.example"); v.Allowed {
				t.Errorf("%s: name resolving to %q pinned", scope, lit)
			}
		}
	}
}

// A02: odd host spellings (decimal, octal, hex, short, trailing dots, unicode dots) never reach a dial,
// even if the system resolver (getaddrinfo) would turn them into loopback.
func TestA02_NonCanonicalHostSpellings(t *testing.T) {
	loop := []netip.Addr{netip.MustParseAddr("127.0.0.1")}
	forms := []string{"2130706433", "0x7f000001", "0x7f.1", "0177.0.0.1", "127.1", "017700000001", "127.0.0.1.", "127.0.0.1..",
		"LOCALHOST.", "localhost", "0", "0.0.0.0.", "10.0.0.1.", "[::1]", "::1.", "[::ffff:127.0.0.1]", "127。0。0。1", "１２７.０.０.１"}
	res := &aFakeResolver{answers: map[string][][]netip.Addr{}}
	for _, f := range forms {
		res.answers[netscope.NormalizeHost(strings.Trim(f, "[]"))] = [][]netip.Addr{loop}
	}
	for _, scope := range []string{"restricted", "internet"} {
		g := netscope.Guard{Scope: aModelScope(scope), Facts: aLanFacts(), Resolver: res}
		for _, f := range forms {
			if addrs, v := g.Pin(context.Background(), f); v.Allowed {
				t.Errorf("%s: host %q pinned %v", scope, f, addrs)
			}
		}
	}
}

// A03: allowlist confusion: suffix, prefix, userinfo, port, case, unicode lookalike and control characters.
func TestA03_AllowlistConfusion(t *testing.T) {
	bad := []string{"registry.npmjs.org.attacker.com", "evil-registry.npmjs.org", "xregistry.npmjs.org", "registry.npmjs.org@evil.com",
		"evil.com#registry.npmjs.org", "evil.com/registry.npmjs.org", "evil.com\\registry.npmjs.org", "registry.npmjs.org\x00.evil.com",
		"registry.npmjs.org\n.evil.com", "registry.npmjs.оrg" /* cyrillic o */, "registry.npmjs.org%2eevil.com", "*.npmjs.org", ".npmjs.org",
		"npmjs.org", "org", "", " ", "registry.npmjs.org evil.com", "pypi.org:80@127.0.0.1"}
	for _, h := range bad {
		if netscope.HostAllowed("restricted", h, nil) {
			t.Errorf("restricted allowed %q", h)
		}
	}
	for _, scope := range []string{"none", "lan", "privileged", "", "bogus"} {
		if netscope.HostAllowed(aModelScope(scope), "registry.npmjs.org", nil) {
			t.Errorf("scope %q allowed a host", scope)
		}
		if v := netscope.Allowed(aModelScope(scope), netip.MustParseAddr("93.184.216.34"), netscope.Facts{}); v.Allowed {
			t.Errorf("scope %q allowed an address", scope)
		}
	}
}

// A04: DNS rebinding and split answers: the name is resolved once, the pinned IP is dialled, and one denied
// address in the answer refuses the whole host.
func TestA04_RebindingAndSplitAnswer(t *testing.T) {
	pub, priv := netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("10.0.0.5")
	res := &aFakeResolver{answers: map[string][][]netip.Addr{
		"rebind.example": {{pub}, {priv}},
		"split.example":  {{pub, priv}},
		"split6.example": {{pub, netip.MustParseAddr("::ffff:10.0.0.5")}},
	}}
	d := &aRecDialer{}
	p := &netscope.Proxy{Guard: netscope.Guard{Scope: "internet", Facts: aLanFacts(), Resolver: res}, Dialer: d}
	ln, stop := aServeProxy(t, p)
	defer stop()
	for i := 0; i < 3; i++ {
		_ = aConnectStatus(t, ln.Addr().String(), "rebind.example:443")
	}
	for _, addr := range d.dialed {
		if strings.HasPrefix(addr, "10.") || strings.Contains(addr, "rebind") {
			t.Errorf("dialled %q after rebinding", addr)
		}
	}
	for _, name := range []string{"split.example", "split6.example"} {
		if st := aConnectStatus(t, ln.Addr().String(), name+":443"); !strings.Contains(st, "403") {
			t.Errorf("split answer %s status %q", name, st)
		}
	}
}

// A05: restricted scope port tricks and a host header that disagrees with the request target.
func TestA05_PortsAndHostHeader(t *testing.T) {
	res := &aFakeResolver{answers: map[string][][]netip.Addr{"registry.npmjs.org": {{netip.MustParseAddr("104.16.1.1")}}}}
	d := &aRecDialer{seen: make(chan *http.Request, 4)}
	p := &netscope.Proxy{Guard: netscope.Guard{Scope: "restricted", Facts: aLanFacts(), Resolver: res}, Dialer: d}
	ln, stop := aServeProxy(t, p)
	defer stop()
	for _, target := range []string{"registry.npmjs.org:22", "registry.npmjs.org:8080", "registry.npmjs.org:0", "registry.npmjs.org:4430",
		"registry.npmjs.org:+443", "registry.npmjs.org:0443", "registry.npmjs.org:443 ", "registry.npmjs.org:https", "registry.npmjs.org:65536"} {
		if st := aConnectStatus(t, ln.Addr().String(), target); strings.Contains(st, " 200") {
			t.Errorf("CONNECT %q tunnelled: %s", target, st)
		}
	}
	if n := d.count(); n != 0 {
		t.Errorf("dialled %d times for refused ports: %v", n, d.dialed)
	}
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, _ = c.Write([]byte("GET http://registry.npmjs.org/x HTTP/1.1\r\nHost: 169.254.169.254\r\nX-Forwarded-Host: 10.0.0.1\r\nConnection: close\r\n\r\n"))
	select {
	case req := <-d.seen:
		if req.Host != "registry.npmjs.org" {
			t.Errorf("upstream saw Host %q, want the checked host", req.Host)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("proxy never forwarded the allowed request")
	}
	if got := d.dialed; len(got) != 1 || got[0] != "104.16.1.1:80" {
		t.Errorf("dial targets %v, want the pinned address only", got)
	}
}

// A06: a dial that lands on a different address than the pinned one (a rebinding resolver inside the dialler,
// a redirect at the network layer) is closed.
func TestA06_ConnectedAddressRechecked(t *testing.T) {
	res := &aFakeResolver{answers: map[string][][]netip.Addr{"ok.example": {{netip.MustParseAddr("93.184.216.34")}}}}
	p := &netscope.Proxy{Guard: netscope.Guard{Scope: "internet", Facts: aLanFacts(), Resolver: res}, Dialer: aLandsOn{"10.9.9.9"}}
	ln, stop := aServeProxy(t, p)
	defer stop()
	if st := aConnectStatus(t, ln.Addr().String(), "ok.example:443"); strings.Contains(st, " 200") {
		t.Errorf("tunnel opened to a connection that landed on a LAN address: %s", st)
	}
}

type aLandsOn struct{ ip string }

func (l aLandsOn) DialContext(context.Context, string, string) (net.Conn, error) {
	c, _ := net.Pipe()
	return aFixedRemote{c, &net.TCPAddr{IP: net.ParseIP(l.ip), Port: 443}}, nil
}

type aFixedRemote struct {
	net.Conn
	addr net.Addr
}

func (f aFixedRemote) RemoteAddr() net.Addr { return f.addr }
