package coordinator

// Purpose: /healthz, the info page and the access log of the V15 coordinator (P7-NODE-10, review F23).
// Inputs:  HTTP requests; the bind address of the listener.
// Outputs: the v1.4 /healthz body on every bind; queue depth, in-flight count and lease count added only
//          on a loopback bind; one access-log line per request.
// Constraints: the access log carries method, path, status and duration only: no header, no query and no
//          body is logged, so Authorization and the signature header never reach a log.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

func newToken() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// isLoopback reports whether a host:port address is bound to the loopback interface only.
func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c *Coordinator) healthz(w http.ResponseWriter, r *http.Request, loopback bool) {
	body := map[string]any{"status": "ok", "service": "nself-ci-serve", "port": "3845"}
	if loopback && localCaller(r) {
		if st, err := c.cfg.Store.IntakeStats(r.Context()); err == nil {
			body["queue_depth"], body["dispatched"], body["leases"] = st.Queued, st.Dispatched, st.Leases
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(body)
}

func (c *Coordinator) info(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(w, "nself-ci-serve\n  port        : 3845\n  concurrency : %d\n  /healthz    : health probe\n  POST /      : GitHub webhook (push, pull_request)\n  POST /v1/trigger : manual trigger (Idempotency-Key)\n", c.cfg.Concurrency)
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (s *statusWriter) WriteHeader(code int) {
	s.code = code
	s.ResponseWriter.WriteHeader(code)
}

// accessLog logs method, path, status and duration of every request.
func accessLog(next http.Handler, logf func(string, ...any)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start, sw := time.Now(), &statusWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(sw, r)
		logf("%s %s %d %s", r.Method, cleanPath(r.URL.Path), sw.code, time.Since(start).Round(time.Millisecond))
	})
}

// cleanPath makes a request path safe for one log line: control characters become '?'.
func cleanPath(p string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '?'
		}
		return r
	}, p)
}

// localCaller says the request came straight from this host and was not relayed by a proxy or tunnel
// (which would add a forwarding header): a relayed caller may be anyone, so it gets the v1.4 body only.
func localCaller(r *http.Request) bool {
	if r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("Forwarded") != "" || r.Header.Get("X-Real-Ip") != "" {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
