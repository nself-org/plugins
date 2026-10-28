package remote

// Purpose: a short-lived cache of the collected release inputs, so signing a release of N blobs
// reads the store once for the whole list instead of once per blob (D-1356).
// Inputs:  the run id and the caller's role (the cache key), a clock for the expiry.
// Outputs: the Inputs last collected for that key, while they are younger than TTL.
// Constraints: the role check and its audit row still run on every request, before the cache is
// asked; the key holds the caller's role because the listed records depend on it (a lower role
// sees fewer records); only a successful collect is stored, never an error; the blob route reads
// the cache, the inputs route always collects afresh and refreshes it; the cache lives in Deps,
// so a coordinator restart starts it empty; an entry is dropped once it is older than TTL, and a
// full cache is emptied rather than grown.

import (
	"context"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

// DefaultInputsTTL is how long a collected list answers blob requests without another collect.
const DefaultInputsTTL = 5 * time.Second

// maxCachedRuns bounds the entries; one entry is at most MaxRecords records plus the evidence body.
const maxCachedRuns = 32

type cacheKey struct {
	run  string
	role authz.Role
}

type cacheEntry struct {
	in Inputs
	at time.Time
}

// InputsCache holds the collected release inputs per (run, role). The zero value is ready and
// uses DefaultInputsTTL and the wall clock; a nil *InputsCache caches nothing.
type InputsCache struct {
	TTL time.Duration    // zero means DefaultInputsTTL
	Now func() time.Time // nil means time.Now

	mu sync.Mutex
	m  map[cacheKey]cacheEntry
}

func (c *InputsCache) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *InputsCache) ttl() time.Duration {
	if c.TTL > 0 {
		return c.TTL
	}
	return DefaultInputsTTL
}

// get returns the cached inputs of (run, role) when they are still fresh.
func (c *InputsCache) get(run string, role authz.Role) (Inputs, bool) {
	if c == nil {
		return Inputs{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	k := cacheKey{run, role}
	e, ok := c.m[k]
	if !ok {
		return Inputs{}, false
	}
	if c.now().Sub(e.at) >= c.ttl() {
		delete(c.m, k)
		return Inputs{}, false
	}
	return e.in, true
}

// put stores freshly collected inputs.
func (c *InputsCache) put(run string, role authz.Role, in Inputs) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if c.m == nil {
		c.m = map[cacheKey]cacheEntry{}
	}
	if len(c.m) >= maxCachedRuns {
		for k, e := range c.m {
			if now.Sub(e.at) >= c.ttl() {
				delete(c.m, k)
			}
		}
		if len(c.m) >= maxCachedRuns {
			clear(c.m)
		}
	}
	c.m[cacheKey{run, role}] = cacheEntry{in: in, at: now}
}

// collectCached answers from the cache when fromCache is set and the entry is fresh, and
// otherwise collects and stores the result.
func (d Deps) collectCached(ctx context.Context, run string, role authz.Role, fromCache bool) (Inputs, error) {
	if fromCache {
		if in, ok := d.Cache.get(run, role); ok {
			return in, nil
		}
	}
	in, err := d.collect(ctx, run, role)
	if err == nil {
		d.Cache.put(run, role, in)
	}
	return in, err
}
