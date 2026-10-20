package remote

// Purpose: a short-lived cache of the collected release inputs, so signing a release of N blobs
// reads the store once instead of N times (D-1356).
// Inputs:  the store, the run id and the caller's role (what collect depends on).
// Outputs: the Inputs last collected for that pair, while younger than the TTL.
// Constraints: only the blob route uses it; the authorization (and its one audit row) and the
// run lookup still happen on every request, so a refused caller never reaches the cache; only a
// successful collect is stored; a run is released only after it ended, so its inputs change
// rarely, and the TTL bounds how long a change can go unseen; at most inputsCacheMax entries,
// the oldest dropped first; safe for concurrent use.

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

const (
	inputsCacheTTL = 5 * time.Second
	inputsCacheMax = 32
)

type inputsKey struct {
	store Backend // the coordinator's store: a restart or a swapped Deps never sees an older store's inputs
	run   string
	role  authz.Role
}

type inputsEntry struct {
	in Inputs
	at time.Time
}

type inputsCache struct {
	mu sync.Mutex
	m  map[inputsKey]inputsEntry
}

func newInputsCache() *inputsCache { return &inputsCache{m: map[inputsKey]inputsEntry{}} }

// collect returns the cached inputs of run for role, or collects and remembers them.
func (c *inputsCache) collect(ctx context.Context, d Deps, run string, role authz.Role) (Inputs, error) {
	if d.Store == nil || !reflect.TypeOf(d.Store).Comparable() { // cannot key on it: no cache
		return d.collect(ctx, run, role)
	}
	key, now := inputsKey{d.Store, run, role}, time.Now()
	c.mu.Lock()
	e, ok := c.m[key]
	c.mu.Unlock()
	if ok && now.Sub(e.at) < inputsCacheTTL {
		return e.in, nil
	}
	in, err := d.collect(ctx, run, role)
	if err != nil {
		return Inputs{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, old := range c.m { // drop what has expired, then the oldest if still full
		if now.Sub(old.at) >= inputsCacheTTL {
			delete(c.m, k)
		}
	}
	for len(c.m) >= inputsCacheMax {
		var oldest inputsKey
		var at time.Time
		for k, v := range c.m {
			if at.IsZero() || v.at.Before(at) {
				oldest, at = k, v.at
			}
		}
		delete(c.m, oldest)
	}
	c.m[key] = inputsEntry{in: in, at: now}
	return in, nil
}
