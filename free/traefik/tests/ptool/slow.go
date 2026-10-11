package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"time"
)

// slowPost sends a body of secs/4 bytes, one byte every 4s. nginx times only the gap between reads
// (client_body_timeout 60s), so any length of upload with steady progress gets 200. Traefik's
// entrypoint readTimeout (60s) is a total deadline for the request, so an upload longer than that
// is cut: the accepted difference. expect=ok needs 200, expect=cut needs anything else.
func slowPost(m routes, name string, secs int, expect string) {
	var rt route
	found := false
	for _, x := range m.Routes {
		if !found && hasRoot(x) && (x.Listen.HTTP || x.Listen.HTTPS) {
			rt, found = x, true
		}
	}
	if !found {
		die(fmt.Errorf("no route with an upstream on /"))
	}
	n := secs / 4
	c, err := dialRaw(target{name}, scheme(rt), hostOf(rt))
	must(err)
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(time.Duration(secs+70) * time.Second))
	fmt.Fprintf(c, "POST / HTTP/1.1\r\nHost: %s\r\nContent-Type: text/plain\r\nContent-Length: %d\r\nConnection: close\r\n\r\n", hostOf(rt), n)
	var werr error
	for i := 0; i < n && werr == nil; i++ {
		_, werr = c.Write([]byte("x"))
		time.Sleep(4 * time.Second)
	}
	status := 0
	if resp, err := http.ReadResponse(bufio.NewReader(c), nil); err == nil {
		status = resp.StatusCode
		resp.Body.Close()
	}
	got := "ok"
	if status != 200 {
		got = "cut"
	}
	fmt.Printf("slow upload (%ds) via %s on %s: status %d (write error %v), %s\n", secs, name, rt.ID, status, werr, got)
	if got != expect {
		die(fmt.Errorf("slow upload of %ds via %s: want %s, got %s", secs, name, expect, got))
	}
}

// slowHeader opens a connection on :80, sends an incomplete request header and then either nothing
// (idle) or one more header line every 20s (trickle). nginx closes it at client_header_timeout
// (60s); Traefik at readTimeout. With readTimeout 0s nothing closes it: a slowloris hold. The
// connection must be closed between 50s and 75s.
func slowHeader(m routes, name, mode string) {
	var rt route
	found := false
	for _, x := range m.Routes {
		if !found && x.Listen.HTTP {
			rt, found = x, true
		}
	}
	if !found {
		die(fmt.Errorf("no route on :80"))
	}
	c, err := dialRaw(target{name}, "http", hostOf(rt))
	must(err)
	defer c.Close()
	start := time.Now()
	fmt.Fprintf(c, "GET / HTTP/1.1\r\nHost: %s\r\n", hostOf(rt))
	closed := make(chan error, 1)
	go func() { _, err := io.Copy(io.Discard, c); closed <- err }()
	tick := time.NewTicker(20 * time.Second)
	defer tick.Stop()
	deadline := time.After(120 * time.Second)
	for i := 1; ; i++ {
		select {
		case <-closed:
			el := time.Since(start).Round(time.Second)
			fmt.Printf("partial header (%s) via %s: connection closed after %s\n", mode, name, el)
			if el < 50*time.Second || el > 75*time.Second {
				die(fmt.Errorf("closed after %s, want about 60s", el))
			}
			return
		case <-tick.C:
			if mode == "trickle" {
				fmt.Fprintf(c, "X-Slow-%d: 1\r\n", i)
			}
		case <-deadline:
			die(fmt.Errorf("partial header (%s) via %s: connection still open after 120s (slowloris hold)", mode, name))
		}
	}
}
