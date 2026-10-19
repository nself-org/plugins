package netscope

// Purpose: the egress proxy that runs inside the per-attempt sidecar (never in the agent or coordinator).
// Inputs:  a net.Listener on the sidecar's job-network address, a Guard, a Dialer and an event writer.
// Outputs: CONNECT tunnels and absolute-form HTTP forwards to pinned public addresses; 403 plus one
//          structured event for every refusal.
// Constraints: CONNECT and absolute-form HTTP only (an origin-form request is a 400: the proxy is not a
// web server); bodies are streamed and never inspected or logged; the destination is resolved once and the
// pinned address is dialled; a client cannot reach the sidecar's own addresses (they are in the deny Facts).

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	maxConns       = 256
	headerTimeout  = 15 * time.Second
	dialTimeout    = 15 * time.Second
	maxHeaderBytes = 64 << 10
)

// Proxy is the sidecar server.
type Proxy struct {
	Guard  Guard
	Dialer Dialer
	Events io.Writer

	mu     sync.Mutex
	sem    chan struct{}
	events int
}

// maxEvents bounds the denial events one sidecar writes, so a job cannot flood the log that the planner reads
// back; the last event states that the rest were dropped. maxEventHost bounds the host a refusal records.
const (
	maxEvents    = 4000
	maxEventHost = 253
)

// Serve accepts on ln until ctx ends.
func (p *Proxy) Serve(ctx context.Context, ln net.Listener) error {
	p.sem = make(chan struct{}, maxConns)
	srv := &http.Server{Handler: http.HandlerFunc(p.handle), ReadHeaderTimeout: headerTimeout, MaxHeaderBytes: maxHeaderBytes, IdleTimeout: 2 * time.Minute}
	go func() { <-ctx.Done(); _ = srv.Close() }()
	if err := srv.Serve(ln); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

func (p *Proxy) deny(w http.ResponseWriter, host, port string, v Verdict) {
	p.record(host, port, v)
	// the body names no reason: the job must not learn which addresses are deploy hosts or on-link
	http.Error(w, "nself ci network scope: destination refused", http.StatusForbidden)
}

// record writes the refusal event unless the cap is reached.
func (p *Proxy) record(host, port string, v Verdict) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Events == nil || p.events > maxEvents {
		return
	}
	p.events++
	e := Event{Scope: string(p.Guard.Scope), Host: clip(host, maxEventHost), Port: clip(port, 5), Reason: v.Reason, Detail: clip(v.Detail, 2*maxEventHost)}
	if p.events > maxEvents {
		e = Event{Scope: string(p.Guard.Scope), Reason: ReasonEventsDropped, Detail: "further denials are not recorded"}
	}
	_, _ = p.Events.Write(e.Encode())
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func (p *Proxy) handle(w http.ResponseWriter, r *http.Request) {
	select {
	case p.sem <- struct{}{}:
		defer func() { <-p.sem }()
	default:
		http.Error(w, "too many connections", http.StatusServiceUnavailable)
		return
	}
	switch {
	case r.Method == http.MethodConnect:
		p.connect(w, r)
	case r.URL != nil && r.URL.IsAbs() && (r.URL.Scheme == "http"):
		p.forward(w, r)
	default:
		http.Error(w, "this is a proxy: use CONNECT or an absolute http URL", http.StatusBadRequest)
	}
}

func splitTarget(hostport, defPort string) (string, string, bool) {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		if strings.Contains(hostport, ":") && !strings.HasPrefix(hostport, "[") {
			return "", "", false // a bare IPv6 literal without brackets is ambiguous
		}
		host, port = strings.Trim(hostport, "[]"), defPort
	}
	if host == "" || port == "" {
		return "", "", false
	}
	return host, port, true
}

func (p *Proxy) connect(w http.ResponseWriter, r *http.Request) {
	host, port, ok := splitTarget(r.Host, "443")
	if !ok {
		http.Error(w, "bad CONNECT target", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), dialTimeout)
	defer cancel()
	addrs, v := p.Guard.Pin(ctx, host)
	if !v.Allowed {
		p.deny(w, host, port, v)
		return
	}
	up, err := p.Guard.dialPinned(ctx, p.Dialer, addrs, port)
	if err != nil {
		p.record(host, port, Verdict{Reason: ReasonDialFailed, Detail: err.Error()})
		http.Error(w, "upstream unreachable", http.StatusBadGateway)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		_ = up.Close()
		http.Error(w, "hijack unsupported", http.StatusInternalServerError)
		return
	}
	down, buf, err := hj.Hijack()
	if err != nil {
		_ = up.Close()
		return
	}
	_ = down.SetDeadline(time.Time{}) // the server's header timeout must not cut a long tunnel
	_, _ = down.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if n := buf.Reader.Buffered(); n > 0 { // bytes the client sent behind the CONNECT line
		b, _ := buf.Reader.Peek(n)
		_, _ = up.Write(b)
	}
	pipe(down, up)
}

// pipe copies both ways and closes both ends when either side finishes.
func pipe(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	cp := func(dst, src net.Conn) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
		if tc, ok := dst.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}
	go cp(a, b)
	go cp(b, a)
	wg.Wait()
	_ = a.Close()
	_ = b.Close()
}

var hopHeaders = []string{"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade"}

func (p *Proxy) forward(w http.ResponseWriter, r *http.Request) {
	host, port, ok := splitTarget(r.URL.Host, "80")
	if !ok {
		http.Error(w, "bad request target", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), dialTimeout)
	addrs, v := p.Guard.Pin(ctx, host)
	cancel()
	if !v.Allowed {
		p.deny(w, host, port, v)
		return
	}
	tr := &http.Transport{
		DisableKeepAlives: true,
		Proxy:             nil, // never chain to another proxy named by the environment
		DialContext: func(ctx context.Context, _ string, _ string) (net.Conn, error) {
			return p.Guard.dialPinned(ctx, p.Dialer, addrs, port) // the pinned addresses, not the name
		},
		ResponseHeaderTimeout: 60 * time.Second,
	}
	out := r.Clone(r.Context())
	out.RequestURI = ""
	for _, h := range hopHeaders {
		out.Header.Del(h)
	}
	resp, err := tr.RoundTrip(out)
	if err != nil {
		p.record(host, port, Verdict{Reason: ReasonDialFailed, Detail: err.Error()})
		http.Error(w, "upstream unreachable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for _, h := range hopHeaders {
		resp.Header.Del(h)
	}
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
