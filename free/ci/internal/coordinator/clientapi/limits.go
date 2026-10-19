package clientapi

// Purpose: the two abuse limits of the user API (P7-NODE-19, Epic D14): a per-source cap on failed
// authentications and a per-principal cap on open streams.
// Inputs:  the remote address of a request, the principal of an actor.
// Outputs: E664 rate_limited (429) when a limit is hit.
// Constraints: a source that failed authFailMax times inside authFailWindow gets 429 before any store
//          lookup until the window passes (a flood of bad tokens costs no database reads); only failed
//          authentications count (a refused role does not); the table holds at most limiterEntries sources
//          and drops the stalest when full; a principal holds at most streamsPerPrincipal events or log
//          streams at once; a stream slot is released when the stream ends.

import (
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	authFailMax         = 30
	authFailWindow      = time.Minute
	limiterEntries      = 4096
	streamsPerPrincipal = 8
	ReasonRateLimited   = "rate_limited"
)

type failCount struct {
	n     int
	since time.Time
}

type limiter struct {
	mu      sync.Mutex
	fails   map[string]*failCount
	streams map[string]int
	now     func() time.Time
}

func newLimiter() *limiter {
	return &limiter{fails: map[string]*failCount{}, streams: map[string]int{}, now: time.Now}
}

func sourceOf(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// allowed reports whether src may try to authenticate.
func (l *limiter) allowed(src string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	f := l.fails[src]
	if f == nil {
		return true
	}
	if l.now().Sub(f.since) > authFailWindow {
		delete(l.fails, src)
		return true
	}
	return f.n < authFailMax
}

// failed records one failed authentication from src.
func (l *limiter) failed(src string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	f := l.fails[src]
	if f == nil || now.Sub(f.since) > authFailWindow {
		if f == nil && len(l.fails) >= limiterEntries {
			var oldest string
			var at time.Time
			for k, v := range l.fails {
				if oldest == "" || v.since.Before(at) {
					oldest, at = k, v.since
				}
			}
			delete(l.fails, oldest)
		}
		f = &failCount{since: now}
		l.fails[src] = f
	}
	f.n++
}

// openStream takes a stream slot for principal; the returned function gives it back.
func (l *limiter) openStream(principal string) (release func(), ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.streams[principal] >= streamsPerPrincipal {
		return nil, false
	}
	l.streams[principal]++
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			if l.streams[principal]--; l.streams[principal] <= 0 {
				delete(l.streams, principal)
			}
		})
	}, true
}
