package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func sample(path, match string) string {
	switch {
	case match == "exact" || path == "/":
		return path
	case strings.HasSuffix(path, "/"):
		return path + "v1/item?q=1"
	}
	return path + "/x"
}

func scheme(rt route) string {
	if rt.Listen.HTTPS {
		return "https"
	}
	return "http"
}

func matrix(m routes, ngx, trf string) {
	r := &runner{n: target{ngx}, t: target{trf}}
	hasTLS := false
	for _, rt := range m.Routes {
		hasTLS = hasTLS || rt.Listen.HTTPS
	}
	get := func(rt route, path string) req {
		return req{scheme: scheme(rt), host: rt.ServerNames[0], method: "GET", path: path}
	}
	compressed := false
	for _, rt := range m.Routes {
		for _, l := range rt.Locations {
			if l.StaticRoot != nil {
				continue
			}
			mode, want := "full", 0
			if strings.Contains(l.Path, "internal") && l.Upstream == nil {
				mode, want = "status", 403
			} else if l.Upstream == nil {
				mode = "loc"
			}
			r.pairMode(fmt.Sprintf("%s GET %s", rt.ID, sample(l.Path, l.Match)), mode, plain(get(rt, sample(l.Path, l.Match))), want)
		}
		for _, p := range []string{"/.env", "/a/.git/HEAD", "/Dockerfile"} {
			if len(rt.BlockedPaths) > 0 {
				r.pairMode(rt.ID+" blocked "+p, "status", plain(get(rt, p)), 404)
			}
		}
		if hasRoot(rt) {
			q := get(rt, "/")
			q.method, q.body, q.hdr = "POST", strings.NewReader("hello parity"), map[string]string{"Content-Type": "text/plain", "X-Notes-Tenant": "client"}
			r.pairMode(rt.ID+" POST /", "full", func() req { c := q; c.body = strings.NewReader("hello parity"); return c }, 0)
			h := get(rt, "/")
			h.method = "HEAD"
			r.pair(rt.ID+" HEAD /", plain(h), 0)
		}
		if rt.Listen.HTTP && hasACME(rt) {
			r.pair(rt.ID+" ACME token", plain(get(rt, "/.well-known/acme-challenge/parity-token")), 200)
			r.pairMode(rt.ID+" ACME bad path", "status", plain(get(rt, "/.well-known/acme-challenge/a/b")), 404)
			p := get(rt, "/.well-known/acme-challenge/parity-token")
			p.method = "POST"
			r.pairMode(rt.ID+" ACME POST", "status", plain(p), 403)
		}
		if rt.Listen.HTTP || rt.Listen.HTTPS {
			for _, l := range rt.Locations {
				if l.Path == "/" && l.WebSocket {
					a, ea := wsCheck(r.n, scheme(rt), rt.ServerNames[0])
					b, eb := wsCheck(r.t, scheme(rt), rt.ServerNames[0])
					if ea != nil || eb != nil || a != b || !strings.HasPrefix(a, "101") {
						r.fail(rt.ID+" websocket", fmt.Sprintf("nginx=%q (%v) traefik=%q (%v)", a, ea, b, eb))
					} else {
						r.ok(rt.ID + " websocket " + a)
					}
				}
			}
		}
		if !compressed && hasRoot(rt) {
			compressed = true
			for _, ext := range []string{"txt", "json", "html", "css", "png"} {
				q := get(rt, "/api/big."+ext)
				q.hdr, q.gz = map[string]string{"Accept-Encoding": "gzip"}, true
				a, b := r.pair(rt.ID+" gzip "+ext, plain(q), 200)
				if want := ext != "png"; (a.hdr.Get("Content-Encoding") == "gzip") != want || (b.hdr.Get("Content-Encoding") == "gzip") != want {
					r.fail(rt.ID+" gzip "+ext, fmt.Sprintf("Content-Encoding nginx=%q traefik=%q, want gzip=%v", a.hdr.Get("Content-Encoding"), b.hdr.Get("Content-Encoding"), want))
				}
			}
		}
	}
	unknownHost(r, m, hasTLS)
	if hasTLS {
		sans(r, m)
	}
	bodyLimit(r, m, get)
	rates(r, m, get)
	fmt.Printf("\nmatrix: %d checks passed, %d failed\n", r.checks, len(r.fails))
	if len(r.fails) > 0 || r.checks == 0 {
		for _, f := range r.fails {
			fmt.Println(" -", f)
		}
		os.Exit(1)
	}
}

func hasRoot(rt route) bool {
	for _, l := range rt.Locations {
		if l.Path == "/" && l.Upstream != nil {
			return true
		}
	}
	return false
}

func hasACME(rt route) bool {
	for _, l := range rt.Locations {
		if l.StaticRoot != nil {
			return true
		}
	}
	return false
}

func backendHits() string {
	resp, err := http.Get("http://backend:9999/hits")
	if err != nil {
		return err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

// unknownHost covers EPIC D25: an unknown Host reaches no upstream on either proxy.
func unknownHost(r *runner, m routes, hasTLS bool) {
	before := backendHits()
	u := func(path string) req { return req{scheme: "http", host: "unknown.invalid", method: "GET", path: path} }
	if m.DefaultServer.HTTP.RedirectHTTPS {
		r.pairMode("unknown host :80 /", "loc", plain(u("/")), 301)
	} else {
		r.pairMode("unknown host :80 /", "body", plain(u("/")), 200)
	}
	r.pairMode("unknown host :80 /health", "body", plain(u("/health")), 200)
	r.pair("unknown host :80 ACME token", plain(u("/.well-known/acme-challenge/parity-token")), 200)
	if hasTLS {
		q := req{scheme: "https", host: "unknown.invalid", method: "GET", path: "/"}
		a, b := r.n.do(q), r.t.do(q)
		// the one accepted difference: nginx closes the connection (return 444), Traefik answers an empty 404
		if b.err != nil || b.status != 404 || b.size != 0 || (a.err == nil && (a.status < 400 || a.size > 0)) {
			r.fail("unknown host :443 /", fmt.Sprintf("nginx=%d/%v traefik=%d/%dB/%v", a.status, a.err, b.status, b.size, b.err))
		} else {
			r.ok(fmt.Sprintf("unknown host :443 / (nginx %v, traefik 404 empty)", a.err))
		}
		q.path = "/health"
		r.pairMode("unknown host :443 /health", "body", plain(q), 200)
	}
	if after := backendHits(); after != before {
		r.fail("unknown host reached an upstream", fmt.Sprintf("backend hits %s -> %s", before, after))
	} else {
		r.ok("unknown host reached no upstream (backend hits " + after + ")")
	}
}

// sans compares the certificate each proxy serves per host, including an unknown SNI.
func sans(r *runner, m routes) {
	seen := map[string]bool{}
	for _, rt := range m.Routes {
		if rt.TLS == nil || seen[rt.ServerNames[0]] {
			continue
		}
		seen[rt.ServerNames[0]] = true
		a, b := r.n.do(req{scheme: "https", host: rt.ServerNames[0], method: "GET", path: "/healthz"}), r.t.do(req{scheme: "https", host: rt.ServerNames[0], method: "GET", path: "/healthz"})
		if strings.Join(a.sans, ",") != strings.Join(b.sans, ",") || len(a.sans) == 0 {
			r.fail("SANs "+rt.ServerNames[0], fmt.Sprintf("nginx=%v traefik=%v", a.sans, b.sans))
		} else {
			r.ok(fmt.Sprintf("SANs %s (%d names)", rt.ServerNames[0], len(a.sans)))
		}
	}
	if a, b := serialOf(r.n.name), serialOf(r.t.name); a != b {
		r.fail("unknown SNI certificate", fmt.Sprintf("nginx serial %s, traefik serial %s", a, b))
	} else {
		r.ok("unknown SNI serves the same default certificate (serial " + a + ")")
	}
}

func serialOf(name string) string {
	s, err := serialErr(name+":443", "unknown.invalid")
	if err != nil {
		return "error: " + err.Error()
	}
	return s
}

func bodyLimit(r *runner, m routes, get func(route, string) req) {
	for _, rt := range m.Routes {
		if !hasRoot(rt) || rt.RateLimit != nil {
			continue
		}
		n := m.Defaults.MaxBodyBytes + 1
		a, ea := declaredTooLarge(r.n, scheme(rt), rt.ServerNames[0], n)
		b, eb := declaredTooLarge(r.t, scheme(rt), rt.ServerNames[0], n)
		if ea != nil || eb != nil || a != 413 || b != 413 {
			r.fail(rt.ID+" body over defaults.max_body_bytes", fmt.Sprintf("nginx=%d (%v) traefik=%d (%v), want 413", a, ea, b, eb))
		} else {
			r.ok(rt.ID + " body over defaults.max_body_bytes: 413 on both")
		}
		return
	}
	r.fail("body limit", "no route to test")
}

// rates fires a rapid burst at one limited location per kind and expects 429 on both.
func rates(r *runner, m routes, get func(route, string) req) {
	done := map[string]bool{}
	for _, rt := range m.Routes {
		type lim struct {
			path  string
			burst int
		}
		var ls []lim
		if rt.RateLimit != nil && hasRoot(rt) {
			ls = append(ls, lim{"/", rt.RateLimit.Burst})
		}
		for _, l := range rt.Locations {
			if l.RateLimit != nil && l.Upstream != nil {
				ls = append(ls, lim{sample(l.Path, l.Match), l.RateLimit.Burst})
			}
		}
		for _, l := range ls {
			key := fmt.Sprint(l.path, l.burst)
			if done[key] {
				continue
			}
			done[key] = true
			perSec := zoneRate(m, zoneOf(rt, l.path))
			var spent [2]time.Duration
			count := func(t target) (first int, ok, limited int) {
				start := time.Now()
				defer func() {
					if t.name == r.n.name {
						spent[0] = time.Since(start)
					} else {
						spent[1] = time.Since(start)
					}
				}()
				for i := 0; i < l.burst+8; i++ {
					res := t.do(get(rt, l.path))
					if i == 0 {
						first = res.status
					}
					if res.status == 429 {
						limited++
					} else {
						ok++ // admitted: any answer that is not the limiter's 429
					}
				}
				return
			}
			// earlier matrix requests drained this bucket: let both fill again so the burst is comparable
			time.Sleep(refillWait(l.burst, perSec))
			fa, oa, la := count(r.n)
			fb, ob, lb := count(r.t)
			label := fmt.Sprintf("%s rate %s burst %d", rt.ID, l.path, l.burst)
			a := burstRun{first: fa, admitted: oa, limited: la, max: allowed(l.burst, perSec, spent[0])}
			b := burstRun{first: fb, admitted: ob, limited: lb, max: allowed(l.burst, perSec, spent[1])}
			if msg := judgeBurst(l.burst, a, b); msg != "" {
				r.fail(label, msg)
			} else {
				r.ok(fmt.Sprintf("%s (admitted nginx=%d traefik=%d, then 429)", label, oa, ob))
			}
			time.Sleep(2 * time.Second)
		}
	}
	if len(done) == 0 {
		r.fail("rate limits", "no limited location in the fixture")
	}
}

// zoneOf returns the zone name limiting path on rt (location zone, else the server zone for "/").
func zoneOf(rt route, path string) string {
	for _, l := range rt.Locations {
		if l.RateLimit != nil && sample(l.Path, l.Match) == path {
			return l.RateLimit.Zone
		}
	}
	if rt.RateLimit != nil {
		return rt.RateLimit.Zone
	}
	return ""
}

// zoneRate is the zone's requests per second ("5r/s", "100r/m").
func zoneRate(m routes, zone string) float64 {
	for _, z := range m.Zones {
		if z.Name == zone && z.Rate != nil {
			var n float64
			var unit string
			if _, err := fmt.Sscanf(*z.Rate, "%fr/%1s", &n, &unit); err == nil {
				if unit == "m" {
					return n / 60
				}
				return n
			}
		}
	}
	return 0
}

// allowed is the most requests a limiter may admit in a burst of elapsed time: the burst allowance
// (1 + burst) plus what the bucket refills meanwhile, plus 2 for scheduling jitter.
func allowed(burst int, perSec float64, elapsed time.Duration) int {
	return burst + 1 + int(perSec*elapsed.Seconds()+0.999) + 2
}
