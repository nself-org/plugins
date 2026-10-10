package main

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

// target is one proxy under test; every request is dialled to it whatever the URL host is.
type target struct{ name string }

type req struct {
	scheme, host, method, path string
	hdr                        map[string]string
	body                       io.Reader
	clen                       int64
	gz                         bool
}

type result struct {
	status int
	hdr    http.Header
	sum    string
	size   int
	sans   []string
	err    error
}

func (t target) client(scheme string) *http.Client {
	port := "80"
	if scheme == "https" {
		port = "443"
	}
	return &http.Client{Timeout: 90 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{DisableCompression: true, DisableKeepAlives: true, TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", t.name+":"+port)
			}}}
}

func (t target) do(q req) result {
	r, err := http.NewRequest(q.method, q.scheme+"://"+q.host+q.path, q.body)
	if err != nil {
		return result{err: err}
	}
	if q.clen > 0 {
		r.ContentLength = q.clen
	}
	for k, v := range q.hdr {
		r.Header.Set(k, v)
	}
	resp, err := t.client(q.scheme).Do(r)
	if err != nil {
		return result{err: err}
	}
	defer resp.Body.Close()
	h := sha256.New()
	var src io.Reader = resp.Body
	if q.gz && resp.Header.Get("Content-Encoding") == "gzip" {
		zr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return result{err: err}
		}
		src = zr
	}
	n, _ := io.Copy(h, src)
	res := result{status: resp.StatusCode, hdr: resp.Header.Clone(), sum: hex.EncodeToString(h.Sum(nil)), size: int(n)}
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		res.sans = resp.TLS.PeerCertificates[0].DNSNames
		sort.Strings(res.sans)
	}
	return res
}

// drop lists headers excluded from comparison: Date and Server per the Ticket,
// plus per-connection framing.
var drop = map[string]bool{"Date": true, "Server": true, "Connection": true, "Keep-Alive": true, "Transfer-Encoding": true}

func norm(h http.Header) []string {
	var out []string
	for k, v := range h {
		// nginx gzip_vary adds Vary: Accept-Encoding to every compressible type; Traefik adds it only when
		// it compresses. Vary is compared when the answer was encoded (the accepted difference, documented).
		if drop[k] || (k == "Content-Length" && h.Get("Content-Encoding") != "") || (k == "Vary" && h.Get("Content-Encoding") == "") {
			continue
		}
		out = append(out, k+": "+strings.Join(v, ", "))
	}
	sort.Strings(out)
	return out
}

// diff compares two results on status, headers (minus Date and Server), body digest and TLS SANs.
func diff(a, b result) string {
	if (a.err != nil) || (b.err != nil) {
		if a.err != nil && b.err != nil {
			return ""
		}
		return fmt.Sprintf("error mismatch: nginx=%v traefik=%v", a.err, b.err)
	}
	var d []string
	if a.status != b.status {
		d = append(d, fmt.Sprintf("status nginx=%d traefik=%d", a.status, b.status))
	}
	if x, y := strings.Join(norm(a.hdr), "\n"), strings.Join(norm(b.hdr), "\n"); x != y {
		d = append(d, "headers:\n  nginx:   "+strings.ReplaceAll(x, "\n", "\n           ")+"\n  traefik: "+strings.ReplaceAll(y, "\n", "\n           "))
	}
	if a.sum != b.sum {
		d = append(d, fmt.Sprintf("body nginx=%s(%dB) traefik=%s(%dB)", a.sum[:8], a.size, b.sum[:8], b.size))
	}
	if strings.Join(a.sans, ",") != strings.Join(b.sans, ",") {
		d = append(d, fmt.Sprintf("SANs nginx=%v traefik=%v", a.sans, b.sans))
	}
	return strings.Join(d, "; ")
}

type runner struct {
	n, t   target
	fails  []string
	checks int
}

func (r *runner) fail(label, why string) {
	r.fails = append(r.fails, label+": "+why)
	fmt.Printf("FAIL %s: %s\n", label, why)
}

func (r *runner) ok(label string) {
	r.checks++
	fmt.Println("ok  ", label)
}

// pair sends the same request to both proxies and compares; mk builds a fresh body per call.
func (r *runner) pair(label string, mk func() req, want int) (result, result) {
	return r.pairMode(label, "full", mk, want)
}

// pairMode compares in one mode: full (status, headers, body, SANs), status, body (status and
// body digest) or loc (status and Location). Synthetic answers (errors, redirects) use the
// narrower modes: only their status and target are contract, not nginx's error page text.
func (r *runner) pairMode(label, mode string, mk func() req, want int) (result, result) {
	a, b := r.n.do(mk()), r.t.do(mk())
	time.Sleep(100 * time.Millisecond)
	d := ""
	switch mode {
	case "full":
		d = diff(a, b)
	default:
		switch {
		case a.err != nil || b.err != nil:
			d = fmt.Sprintf("transport error nginx=%v traefik=%v", a.err, b.err)
		case a.status != b.status:
			d = fmt.Sprintf("status nginx=%d traefik=%d", a.status, b.status)
		case mode == "body" && a.sum != b.sum:
			d = fmt.Sprintf("body nginx=%s traefik=%s", a.sum[:8], b.sum[:8])
		case mode == "loc" && a.hdr.Get("Location") != b.hdr.Get("Location"):
			d = fmt.Sprintf("Location nginx=%q traefik=%q", a.hdr.Get("Location"), b.hdr.Get("Location"))
		}
	}
	if d != "" {
		r.fail(label, d)
	} else if want != 0 && (a.status != want || b.status != want) {
		r.fail(label, fmt.Sprintf("want status %d, got nginx=%d traefik=%d", want, a.status, b.status))
	} else {
		r.ok(label)
	}
	return a, b
}

func plain(q req) func() req { return func() req { return q } }

// wsCheck performs an upgrade handshake and one echoed frame-less exchange against t.
func dialRaw(t target, scheme, host string) (net.Conn, error) {
	port := map[string]string{"http": "80", "https": "443"}[scheme]
	c, err := net.DialTimeout("tcp", t.name+":"+port, 5*time.Second)
	if err != nil {
		return nil, err
	}
	if scheme == "https" {
		c = tls.Client(c, &tls.Config{InsecureSkipVerify: true, ServerName: host})
	}
	return c, nil
}

// declaredTooLarge sends only the headers of a POST whose Content-Length is over the limit and returns the status.
func declaredTooLarge(t target, scheme, host string, n int64) (int, error) {
	c, err := dialRaw(t, scheme, host)
	if err != nil {
		return 0, err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))
	fmt.Fprintf(c, "POST / HTTP/1.1\r\nHost: %s\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\nConnection: close\r\n\r\n", host, n)
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		return 0, err
	}
	return resp.StatusCode, nil
}

func wsCheck(t target, scheme, host string) (string, error) {
	c, err := dialRaw(t, scheme, host)
	if err != nil {
		return "", err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	fmt.Fprintf(c, "GET /ws HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", host, base64.StdEncoding.EncodeToString([]byte("parity-ws-nonce1")))
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 101 {
		return fmt.Sprintf("status %d", resp.StatusCode), nil
	}
	fmt.Fprint(c, "ping-parity\n")
	line, err := br.ReadString('\n')
	return fmt.Sprintf("101 accept=%s echo=%q", resp.Header.Get("Sec-WebSocket-Accept"), strings.TrimSpace(line)), err
}
