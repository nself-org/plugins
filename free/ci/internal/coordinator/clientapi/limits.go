package clientapi

// Purpose: the two abuse limits of the user API (P7-NODE-19, Epic D14): a per-source cap on failed
// authentications and a per-principal cap on open streams.
// Inputs:  the remote address of a request, the principal of an actor.
// Outputs: E664 rate_limited (429) when a limit is hit.
// Constraints: a principal may attempt submitsPerMinute submits a minute, allowed or not (denials are audited
//          rows, so they must not be a way to flood the log); a source that failed authFailMax times inside authFailWindow gets 429 before any store
//          lookup until the window passes (a flood of bad tokens costs no database reads); only failed
//          authentications count (a refused role does not); the table holds at most limiterEntries sources
//          and drops the stalest when full; a principal holds at most streamsPerPrincipal events or log
//          streams at once; a stream slot is released when the stream ends; an actor may be refused a route's role
//          deniesPerMinute times a minute (each refusal is a chained audit row), then gets 429 before the
//          role check writes another.

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
	submitsPerMinute    = 30
	deniesPerMinute     = 30
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
	submits map[string]*failCount
	denies  map[string]*failCount
	now     func() time.Time
}

func newLimiter() *limiter {
	return &limiter{fails: map[string]*failCount{}, streams: map[string]int{}, submits: map[string]*failCount{}, denies: map[string]*failCount{}, now: time.Now}
}

// sourceOf is the address a request counts against: an IPv4 address, or the /64 of an IPv6 one (a host
// owns a whole /64, so rotating inside it must not reset its count).
func sourceOf(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
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

// submitAllowed counts one submit attempt of principal (allowed or not, so denials cannot be used to flood
// the audit log) and reports whether it is within submitsPerMinute.
func (l *limiter) submitAllowed(principal string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	c := l.submits[principal]
	if c == nil || now.Sub(c.since) > time.Minute {
		if c == nil && len(l.submits) >= limiterEntries {
			for k := range l.submits { // drop one arbitrary entry: the table stays bounded
				delete(l.submits, k)
				break
			}
		}
		c = &failCount{since: now}
		l.submits[principal] = c
	}
	c.n++
	return c.n <= submitsPerMinute
}

// denyAllowed reports whether principal may still be refused (and so audited) inside the current minute.
func (l *limiter) denyAllowed(principal string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	c := l.denies[principal]
	if c == nil {
		return true
	}
	if l.now().Sub(c.since) > time.Minute {
		delete(l.denies, principal)
		return true
	}
	return c.n < deniesPerMinute
}

// denied records one role refusal of principal.
func (l *limiter) denied(principal string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	c := l.denies[principal]
	if c == nil || now.Sub(c.since) > time.Minute {
		if c == nil && len(l.denies) >= limiterEntries {
			for k := range l.denies { // drop one arbitrary entry: the table stays bounded
				delete(l.denies, k)
				break
			}
		}
		c = &failCount{since: now}
		l.denies[principal] = c
	}
	c.n++
}
