package health

// Purpose: the tracker keeps one Breaker per provider API family, feeds them from rest outcomes,
//          persists their state in the host-provider store and answers providers.HealthOf
//          (P7-HOST-11, cap:ci.provider-health).
// Inputs:  rest.Outcome values (Tracker.Observe), a provider id (Tracker.HealthOf), a Store.
// Outputs: providers.Health per provider id; a fact host-provider/<family>/health/breaker that
//          `ci providers show` in another process reads; report.Drain on every down -> up move.
// Constraints: the API family of a provider id is the part before the first "-" (github-hosted and
//          github-jit share the "github" API), so one outage covers every provider that talks to
//          that API; the observer path never blocks on the store for more than storeTimeout and
//          never fails (a failed save is counted in SaveErrors); a process that never observed an
//          outcome reads the persisted state, refreshed at most once a second; the drain runs in
//          its own goroutine, one at a time, so a slow status post never holds a request.

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const (
	factScope    = "health"
	factKey      = "breaker"
	storeTimeout = 2 * time.Second
	drainTimeout = 2 * time.Minute
	reloadEvery  = time.Second
)

// Store is the part of the host-provider store the tracker uses; *store.Store satisfies it.
type Store interface {
	ProviderFactPut(ctx context.Context, f store.ProviderFact) (bool, error)
	ProviderFactGet(ctx context.Context, provider, scope, key string) (store.ProviderFact, bool, error)
}

// Options configure a Tracker; every field is optional.
type Options struct {
	Clock func() time.Time
	Store Store
	// Drain delivers the status outbox after a recovery; nil does nothing.
	Drain func(context.Context) (int, error)
	// Ignore names rest client providers the tracker does not track (not provider APIs).
	Ignore []string
}

// Tracker is the health book of every provider API.
type Tracker struct {
	mu       sync.Mutex
	clock    func() time.Time
	store    Store
	drain    func(context.Context) (int, error)
	ignore   map[string]bool
	breakers map[string]*Breaker
	loaded   map[string]time.Time
	drainMu  sync.Mutex
	saveMu   sync.Mutex // saves are ordered: each reads the newest snapshot under it
	wg       sync.WaitGroup
	saveErrs atomic.Int64
	drains   atomic.Int64
}

// New returns a tracker.
func New(o Options) *Tracker {
	t := &Tracker{clock: o.Clock, store: o.Store, drain: o.Drain, ignore: map[string]bool{}, breakers: map[string]*Breaker{}, loaded: map[string]time.Time{}}
	if t.clock == nil {
		t.clock = time.Now
	}
	for _, name := range o.Ignore {
		t.ignore[name] = true
	}
	return t
}

// Family is the API family of a provider id: "github-hosted" -> "github".
func Family(id string) string {
	if i := strings.IndexByte(id, '-'); i > 0 {
		return id[:i]
	}
	return id
}

// SetStore points the tracker at the coordinator's store (nil detaches it).
func (t *Tracker) SetStore(s Store) {
	t.mu.Lock()
	t.store, t.loaded = s, map[string]time.Time{}
	t.mu.Unlock()
}

// SaveErrors counts the saves that failed; the observer path ignores them.
func (t *Tracker) SaveErrors() int64 { return t.saveErrs.Load() }

// Drains counts the outbox drains started by a recovery.
func (t *Tracker) Drains() int64 { return t.drains.Load() }

// Wait blocks until every drain started so far has ended (tests and shutdown).
func (t *Tracker) Wait() { t.wg.Wait() }

func (t *Tracker) breaker(family string) (*Breaker, Store) {
	t.mu.Lock()
	defer t.mu.Unlock()
	b := t.breakers[family]
	if b == nil {
		b = NewBreaker()
		t.breakers[family] = b
	}
	return b, t.store
}

// Observe feeds one request outcome to the breaker of its provider API.
func (t *Tracker) Observe(o rest.Outcome) {
	if o.Provider == "" || t.ignore[o.Provider] {
		return
	}
	fam := Family(o.Provider)
	b, st := t.breaker(fam)
	if st != nil && !b.Observed() { // the first outcome after a restart must start from the saved state
		t.reload(context.Background(), st, fam, b)
	}
	recovered, changed := b.Observe(o, t.clock())
	if changed && st != nil {
		t.save(st, fam, b)
	}
	if recovered && t.drain != nil {
		t.startDrain()
	}
}

// HealthOf is the registered providers health function.
func (t *Tracker) HealthOf(ctx context.Context, id string) providers.Health {
	fam := Family(id)
	b, st := t.breaker(fam)
	if st != nil && !b.Observed() {
		t.reload(ctx, st, fam, b)
	}
	return b.Health(t.clock())
}

// Breaker returns the breaker of a provider id's API (tests and `ci providers show`).
func (t *Tracker) Breaker(id string) *Breaker { b, _ := t.breaker(Family(id)); return b }

// Load reads the persisted state of a provider API into its breaker, if this process has not
// observed any outcome for it.
func (t *Tracker) Load(ctx context.Context, id string) {
	fam := Family(id)
	b, st := t.breaker(fam)
	if st != nil {
		t.reload(ctx, st, fam, b)
	}
}

func (t *Tracker) reload(ctx context.Context, st Store, fam string, b *Breaker) {
	now := t.clock()
	t.mu.Lock()
	last, ok := t.loaded[fam]
	if ok && now.Sub(last) < reloadEvery {
		t.mu.Unlock()
		return
	}
	t.loaded[fam] = now
	t.mu.Unlock()
	c, stop := context.WithTimeout(ctx, storeTimeout)
	defer stop()
	f, found, err := st.ProviderFactGet(c, fam, factScope, factKey)
	if err != nil || !found {
		return
	}
	var s Snapshot
	if json.Unmarshal(f.Value, &s) == nil {
		b.Restore(s)
	}
}

func (t *Tracker) save(st Store, fam string, b *Breaker) {
	t.saveMu.Lock()
	defer t.saveMu.Unlock()
	raw, err := json.Marshal(b.Snapshot())
	if err != nil {
		t.saveErrs.Add(1)
		return
	}
	c, stop := context.WithTimeout(context.Background(), storeTimeout)
	defer stop()
	if _, err = st.ProviderFactPut(c, store.ProviderFact{Provider: fam, Scope: factScope, Key: factKey, Value: raw, ObservedAt: t.clock()}); err != nil {
		t.saveErrs.Add(1)
	}
}

func (t *Tracker) startDrain() {
	t.drains.Add(1)
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.drainMu.Lock()
		defer t.drainMu.Unlock()
		c, stop := context.WithTimeout(context.Background(), drainTimeout)
		defer stop()
		_, _ = t.drain(c) // an undelivered status stays in the outbox for the next drain
	}()
}
