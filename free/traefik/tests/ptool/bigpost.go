package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// patternReader yields n deterministic bytes (position-derived, so the sender and the checker
// agree without storing them).
type patternReader struct{ i, n int64 }

func (p *patternReader) Read(b []byte) (int, error) {
	if p.i >= p.n {
		return 0, io.EOF
	}
	k := int64(len(b))
	if rest := p.n - p.i; k > rest {
		k = rest
	}
	for j := int64(0); j < k; j++ {
		x := p.i + j
		b[j] = byte(x*131 + x>>8 + x>>17)
	}
	p.i += k
	return int(k), nil
}

// bigRoute picks the route bigpost uses: a route with an upstream "/" and no rate limit on it
// (one request must not be throttled), else the slowpost route.
func bigRoute(m routes) (route, int64, bool) {
	for _, x := range m.Routes {
		if !hasRoot(x) || !(x.Listen.HTTP || x.Listen.HTTPS) || x.RateLimit != nil {
			continue
		}
		for _, l := range x.Locations {
			if l.Path == "/" && l.Match == "prefix" && l.Upstream != nil && l.RateLimit == nil {
				limit := m.Defaults.MaxBodyBytes
				if l.MaxBodyBytes != nil {
					limit = *l.MaxBodyBytes
				}
				return x, limit, true
			}
		}
	}
	return route{}, 0, false
}

// bigPost POSTs mib MiB of deterministic bytes to an upstream location and requires a 200 whose
// echoed body_len and body_sha256 match what was sent. Over 1 MiB the Traefik buffering middleware
// spills the body to /tmp, so on the read-only container this proves the tmpfs path. A size over
// the location's body limit is skipped (it would be a 413).
func bigPost(m routes, name string, mib int) {
	rt, limit, ok := bigRoute(m)
	if !ok {
		fmt.Println("bigpost: SKIP no unthrottled upstream / location in the fixture")
		return
	}
	size := int64(mib) << 20
	if limit > 0 && size > limit {
		fmt.Printf("bigpost: SKIP %d MiB is over the body limit %d of %s\n", mib, limit, rt.ID)
		return
	}
	c, err := dialRaw(target{name}, scheme(rt), hostOf(rt))
	must(err)
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(120 * time.Second))
	fmt.Fprintf(c, "POST / HTTP/1.1\r\nHost: %s\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\nConnection: close\r\n\r\n", hostOf(rt), size)
	h := sha256.New()
	_, err = io.Copy(c, io.TeeReader(&patternReader{n: size}, h))
	must(err)
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	must(err)
	defer resp.Body.Close()
	var e echo
	if err := json.NewDecoder(resp.Body).Decode(&e); resp.StatusCode != 200 || err != nil {
		die(fmt.Errorf("bigpost %d MiB via %s on %s: status %d, decode %v", mib, name, rt.ID, resp.StatusCode, err))
	}
	want := hex.EncodeToString(h.Sum(nil))
	if e.BodyLen != size || e.BodySHA != want {
		die(fmt.Errorf("bigpost %d MiB via %s on %s: backend saw %d bytes sha %s, sent %d bytes sha %s", mib, name, rt.ID, e.BodyLen, e.BodySHA, size, want))
	}
	fmt.Printf("bigpost %d MiB via %s on %s: 200, backend saw the same %d bytes and sha256\n", mib, name, rt.ID, size)
}

// uploadOnly checks the upload port on a proxy: the route slowpost uses is served there, every
// other route and an unknown Host get the proxy's own 404 (no default server on that entrypoint).
func uploadOnly(m routes, name, port string) {
	named, ok := slowRoute(m)
	if !ok {
		die(fmt.Errorf("no route with an upstream on /"))
	}
	get := func(sc, host string) int {
		c, err := dialPort(target{name}, sc, host, port)
		must(err)
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(15 * time.Second))
		fmt.Fprintf(c, "GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", host)
		resp, err := http.ReadResponse(bufio.NewReader(c), nil)
		must(err)
		resp.Body.Close()
		return resp.StatusCode
	}
	if st := get(scheme(named), hostOf(named)); st == 404 {
		die(fmt.Errorf("named route %s answers 404 on %s:%s", named.ID, name, port))
	}
	n := 0
	for _, rt := range m.Routes {
		if rt.ID == named.ID || !(rt.Listen.HTTP || rt.Listen.HTTPS) {
			continue
		}
		if st := get(scheme(rt), hostOf(rt)); st != 404 {
			die(fmt.Errorf("route %s is not named but answers %d on %s:%s, want 404", rt.ID, st, name, port))
		}
		n++
	}
	if st := get("http", "unknown.invalid"); st != 404 {
		die(fmt.Errorf("unknown host answers %d on %s:%s, want 404", st, name, port))
	}
	fmt.Printf("uploadonly via %s:%s: %s served, %d other routes and an unknown host answer 404\n", name, port, named.ID, n)
}
