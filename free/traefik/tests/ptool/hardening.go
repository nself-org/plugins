package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// rawStatus sends one request line exactly as given (no client-side cleaning or
// re-encoding of the path) and returns the response status.
func rawStatus(t target, scheme, host, rawPath string) (int, error) {
	c, err := dialRaw(t, scheme, host)
	if err != nil {
		return 0, err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))
	fmt.Fprintf(c, "GET %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", rawPath, host)
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

// encodedSlash covers the %2F bypass: nginx decodes the path and merges it before
// location matching, so a blocked path or deny_all prefix holds. Traefik matches the
// still-encoded path unless entryPoints refuse an encoded slash (allowEncodedSlash:
// false -> 400, the accepted difference). The upstream must never answer.
func encodedSlash(r *runner, m routes) {
	before := backendHits()
	n := 0
	for _, rt := range m.Routes {
		if !rt.Listen.HTTP && !rt.Listen.HTTPS {
			continue
		}
		var paths []string
		if len(rt.BlockedPaths) > 0 {
			paths = append(paths, "/x/..%2f.env", "/%2FDockerfile")
		}
		for _, l := range rt.Locations {
			if l.DenyAll && l.Match == "prefix" && strings.HasPrefix(l.Path, "/") {
				paths = append(paths, "/a/..%2f"+strings.TrimPrefix(l.Path, "/")+"x", "/%2F"+strings.TrimPrefix(l.Path, "/")+"x")
			}
		}
		for _, p := range paths {
			label := rt.ID + " encoded slash " + p
			a, ea := rawStatus(r.n, scheme(rt), hostOf(rt), p)
			b, eb := rawStatus(r.t, scheme(rt), hostOf(rt), p)
			switch {
			case ea != nil || eb != nil:
				r.fail(label, fmt.Sprintf("transport error nginx=%v traefik=%v", ea, eb))
			case a < 400:
				r.fail(label, fmt.Sprintf("reference is not blocking: nginx=%d", a))
			case b != 400 && b != a:
				r.fail(label, fmt.Sprintf("nginx=%d traefik=%d: the encoded slash got past the block", a, b))
			default:
				n++
				r.ok(fmt.Sprintf("%s (nginx %d, traefik %d)", label, a, b))
			}
		}
	}
	if n == 0 {
		r.fail("encoded slash", "no blocked path or deny_all location in the fixture")
	}
	if after := backendHits(); after != before {
		r.fail("encoded slash reached an upstream", fmt.Sprintf("backend hits %s -> %s", before, after))
	}
}

// connRoute is the route whose limit_conn the two-client case exercises: a conn_limit
// and a websocket location on "/" (an upgrade holds its connection open).
func connRoute(m routes) (route, bool) {
	for _, rt := range m.Routes {
		if rt.RateLimit != nil && rt.RateLimit.ConnLimit > 0 && (rt.Listen.HTTP || rt.Listen.HTTPS) {
			for _, l := range rt.Locations {
				if l.Path == "/" && l.WebSocket {
					return rt, true
				}
			}
		}
	}
	return route{}, false
}

func wsOpen(t target, scheme, host string) (net.Conn, error) {
	c, err := dialRaw(t, scheme, host)
	if err != nil {
		return nil, err
	}
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))
	fmt.Fprintf(c, "GET /ws HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n",
		host, base64.StdEncoding.EncodeToString([]byte("parity-hold-nonc")))
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil || resp.StatusCode != 101 {
		c.Close()
		return nil, fmt.Errorf("upgrade: status %v err %v", resp, err)
	}
	_ = c.SetDeadline(time.Time{})
	return c, nil
}

// connHold is client A: it holds conn_limit upgrades open, then must be refused on the
// next request (429 on both proxies), and keeps the connections for secs seconds.
// Exit 3 prints SKIP when the fixture has no such route.
func connHold(m routes, name string, secs int) {
	rt, ok := connRoute(m)
	if !ok {
		fmt.Println("SKIP no route with conn_limit and a websocket on /")
		os.Exit(3)
	}
	t := target{name}
	var held []net.Conn
	for i := 0; i < rt.RateLimit.ConnLimit; i++ {
		c, err := wsOpen(t, scheme(rt), hostOf(rt))
		must(err)
		held = append(held, c)
	}
	fmt.Printf("HELD %d on %s via %s\n", len(held), rt.ID, name)
	st, err := rawStatus(t, scheme(rt), hostOf(rt), "/")
	must(err)
	fmt.Printf("A over limit: %d\n", st)
	if st != 429 {
		die(fmt.Errorf("client A's request over conn_limit %d got %d, want 429", rt.RateLimit.ConnLimit, st))
	}
	time.Sleep(time.Duration(secs) * time.Second)
}

// connProbe is client B, another source address: while A is at the limit it must be served.
func connProbe(m routes, name string) {
	rt, ok := connRoute(m)
	if !ok {
		fmt.Println("SKIP no route with conn_limit and a websocket on /")
		os.Exit(3)
	}
	for _, p := range []string{"/", "/v1/x"} {
		st, err := rawStatus(target{name}, scheme(rt), hostOf(rt), p)
		must(err)
		if st != 200 {
			die(fmt.Errorf("client B GET %s on %s via %s: %d, want 200 (limit is shared across clients)", p, rt.ID, name, st))
		}
		fmt.Printf("client B GET %s via %s: %d\n", p, name, st)
	}
}

// hostOf is a concrete Host header for a route: "*.x" becomes "w.x", ".x" becomes "x".
func hostOf(rt route) string {
	n := rt.ServerNames[0]
	switch {
	case strings.HasPrefix(n, "*."):
		return "w" + n[1:]
	case strings.HasPrefix(n, "."):
		return n[1:]
	}
	return n
}

// hostTier: nginx picks the server by exact name before any wildcard, then the
// location. For every exact host that a wildcard route also matches, each wildcard
// location (path longer than "/") must answer the same on both proxies, i.e. the
// exact host's own "/" upstream, not the wildcard's.
func hostTier(r *runner, m routes, need bool) {
	n := 0
	for _, w := range m.Routes {
		if !strings.HasPrefix(w.ServerNames[0], "*.") || !w.Listen.HTTP {
			continue
		}
		suffix := w.ServerNames[0][1:]
		for _, e := range m.Routes {
			h := e.ServerNames[0]
			if e.ID == w.ID || !e.Listen.HTTP || !strings.HasSuffix(h, suffix) || strings.HasPrefix(h, "*") || !hasRoot(e) {
				continue
			}
			for _, l := range w.Locations {
				if l.Upstream == nil || l.Path == "/" || l.Match != "prefix" || hasLocation(e, l.Path) {
					continue
				}
				q := req{scheme: "http", host: h, method: "GET", path: sample(l.Path, l.Match)}
				label := fmt.Sprintf("host tier %s%s (exact %s beats %s)", h, q.path, e.ID, w.ID)
				a, b := r.n.do(q), r.t.do(q)
				switch d := diff(a, b); {
				case a.status == 429 || b.status == 429: // earlier matrix traffic drained this bucket: nothing to compare
				case d != "":
					r.fail(label, d)
				default:
					n++
					r.ok(label)
				}
			}
		}
	}
	if need && n == 0 {
		r.fail("host tier", "no exact host overlapped by a wildcard route with a longer path")
	}
}

func hasLocation(rt route, path string) bool {
	for _, l := range rt.Locations {
		if l.Path == path {
			return true
		}
	}
	return false
}

// wsIdle holds one websocket idle for secs and then needs an echo: an entrypoint
// read deadline (Traefik v3 defaults to 60s) must not cut it.
func wsIdle(m routes, name string, secs int) {
	var rt route
	found := false
	for _, x := range m.Routes {
		for _, l := range x.Locations {
			if l.Path == "/" && l.WebSocket && !found {
				rt, found = x, true
			}
		}
	}
	if !found {
		die(fmt.Errorf("no route with a websocket on /"))
	}
	c, err := wsOpen(target{name}, scheme(rt), hostOf(rt))
	must(err)
	defer c.Close()
	fmt.Printf("OPEN %s on %s via %s\n", rt.ID, scheme(rt), name)
	time.Sleep(time.Duration(secs) * time.Second)
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	fmt.Fprint(c, "idle-ping\n")
	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "idle-ping" {
		die(fmt.Errorf("websocket via %s did not survive %ds idle: %q %v", name, secs, line, err))
	}
	fmt.Printf("websocket via %s survived %ds idle\n", name, secs)
}

// hostCase: nginx lowercases the Host header before choosing a server, and Traefik's Host() and
// HostRegexp() must too: an upper-case Host reaches the same route on both.
func hostCase(r *runner, m routes) {
	n := 0
	for _, rt := range m.Routes {
		if !hasRoot(rt) || !(rt.Listen.HTTP || rt.Listen.HTTPS) {
			continue
		}
		q := req{scheme: scheme(rt), host: strings.ToUpper(hostOf(rt)), method: "GET", path: "/"}
		a, b := r.n.do(q), r.t.do(q)
		switch {
		case a.err != nil || b.err != nil:
			r.fail(rt.ID+" upper-case Host", fmt.Sprintf("transport error nginx=%v traefik=%v", a.err, b.err))
		case a.status == 429 || b.status == 429: // earlier traffic drained this bucket: nothing to compare
		case a.status != b.status:
			r.fail(rt.ID+" upper-case Host", fmt.Sprintf("status nginx=%d traefik=%d", a.status, b.status))
		default:
			n++
			r.ok(fmt.Sprintf("%s upper-case Host %s -> %d", rt.ID, q.host, a.status))
		}
	}
	if n == 0 {
		r.fail("upper-case Host", "no route compared")
	}
}

// healthMethod: nginx forces GET upstream on a health_probe location, so a DELETE there is
// answered 200 by the upstream's health endpoint. Traefik cannot rewrite the method, so it answers
// 403 and never forwards it (an accepted difference, wiki). Asserted on Traefik only.
func healthMethod(r *runner, m routes) {
	n := 0
	for _, rt := range m.Routes {
		for _, l := range rt.Locations {
			if !l.HealthProbe || l.Upstream == nil || !(rt.Listen.HTTP || rt.Listen.HTTPS) {
				continue
			}
			for _, method := range []string{"DELETE", "POST", "PUT"} {
				q := req{scheme: scheme(rt), host: hostOf(rt), method: method, path: l.Path, body: strings.NewReader("x=1")}
				a, b := r.n.do(q), r.t.do(q)
				if b.err != nil || b.status != 403 {
					r.fail(fmt.Sprintf("%s %s %s", rt.ID, method, l.Path), fmt.Sprintf("Traefik must answer 403 (nginx answered %d); got status %d err %v", a.status, b.status, b.err))
					continue
				}
				n++
				r.ok(fmt.Sprintf("%s %s %s -> traefik 403 (nginx %d, documented)", rt.ID, method, l.Path, a.status))
			}
		}
	}
	if n == 0 {
		r.fail("health probe methods", "no health_probe location compared")
	}
}
