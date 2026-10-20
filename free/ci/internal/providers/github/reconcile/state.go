package reconcile

// Purpose: what the Reconcilers remember in the host-provider store: marks (a head enqueued, a delivery
// redelivered, a job handed to the demand handler), ETags and repository facts, the hourly budget ledger
// and the quota backoff.
// Inputs:  State (the store), the injected clock and jitter.
// Outputs: helpers on Engine; every value is opaque JSON or a time and never carries a token.
// Constraints: a mark never moves back (the store enforces it); the ledger and the backoff are cached in
//          memory and written when they change; a store error is returned, never swallowed, so a pass that
//          cannot remember what it did stops instead of repeating it.

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// StoredFact is the store's fact row.
type StoredFact = store.ProviderFact

var _ State = (*store.Store)(nil)

const (
	markName   = "github-reconcile"
	scopeToken = "_token" // facts about the credential, not a repository
	backoffMin = 30 * time.Second
	backoffMax = 15 * time.Minute
)

func lower(repo string) string { return strings.ToLower(repo) }

// marked reports whether the mark exists.
func (e *Engine) marked(ctx context.Context, kind, repo, id string) (bool, error) {
	s := e.store()
	if s == nil {
		return false, errNoStore
	}
	_, ok, err := s.ProviderMarkGet(ctx, markName, kind+"|"+lower(repo)+"|"+id)
	return ok, err
}

func (e *Engine) mark(ctx context.Context, kind, repo, id string) error {
	s := e.store()
	if s == nil {
		return errNoStore
	}
	return s.ProviderMarkPut(ctx, markName, kind+"|"+lower(repo)+"|"+id, e.now())
}

// putFact stores a JSON value under (repo scope, key).
func (e *Engine) putFact(ctx context.Context, scope, key string, v any) error {
	s := e.store()
	if s == nil {
		return errNoStore
	}
	_, err := s.ProviderFactPut(ctx, StoredFact{Provider: Source, Scope: lower(scope), Key: key, Value: marshal(v), ObservedAt: e.now()})
	return err
}

func (e *Engine) getFact(ctx context.Context, scope, key string, into any) (bool, error) {
	s := e.store()
	if s == nil {
		return false, errNoStore
	}
	f, ok, err := s.ProviderFactGet(ctx, Source, lower(scope), key)
	if err != nil || !ok {
		return false, err
	}
	return json.Unmarshal(f.Value, into) == nil, nil
}

type etagRow struct {
	ETag string `json:"etag"`
}

// etag returns the stored ETag of (repo, key), "" when none.
func (e *Engine) etag(ctx context.Context, repo, key string) (string, error) {
	var row etagRow
	_, err := e.getFact(ctx, repo, "etag:"+key, &row)
	return row.ETag, err
}

func (e *Engine) setEtag(ctx context.Context, repo, key, etag string) error {
	if etag == "" {
		return nil
	}
	return e.putFact(ctx, repo, "etag:"+key, etagRow{ETag: etag})
}

// ledger counts the non-304 requests of the current hour, split by what spent them.
type ledger struct {
	Hour   int64 `json:"hour"`
	Scan   int   `json:"scan"`
	Demand int   `json:"demand"`
	loaded bool
	dirty  bool
}

// Kinds of spending.
const (
	kindScan   = "scan"
	kindDemand = "demand"
)

// ensureLedger loads the stored ledger once.
func (e *Engine) ensureLedger(ctx context.Context) {
	e.mu.Lock()
	loaded := e.ledger.loaded
	e.mu.Unlock()
	if loaded {
		return
	}
	var l ledger
	ok, _ := e.getFact(ctx, scopeToken, "ledger", &l)
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.ledger.loaded {
		e.ledger.loaded = true
		if ok {
			e.ledger.Hour, e.ledger.Scan, e.ledger.Demand = l.Hour, l.Scan, l.Demand
		}
	}
}

// roll starts a new ledger when the hour changed. Callers hold e.mu.
func (e *Engine) roll() {
	if h := e.now().Unix() / 3600; h != e.ledger.Hour {
		e.ledger.Hour, e.ledger.Scan, e.ledger.Demand, e.ledger.dirty = h, 0, 0, true
	}
}

// allow reports whether kind may make another counted request this hour. The hourly allowance is hard
// for both kinds; head scans may use only 60% of it, which keeps the rest for demand polling.
func (e *Engine) allow(ctx context.Context, kind string) bool {
	e.ensureLedger(ctx)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.roll()
	if e.ledger.Scan+e.ledger.Demand >= e.HourlyBudget {
		return false
	}
	return kind != kindScan || e.ledger.Scan < e.HourlyBudget*60/100
}

func (e *Engine) spend(ctx context.Context, kind string) {
	e.ensureLedger(ctx)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.roll()
	if kind == kindScan {
		e.ledger.Scan++
	} else {
		e.ledger.Demand++
	}
	e.ledger.dirty = true
}

// flush writes the ledger when it changed.
func (e *Engine) flush(ctx context.Context) error {
	e.mu.Lock()
	l := e.ledger
	e.ledger.dirty = false
	e.mu.Unlock()
	if !l.dirty {
		return nil
	}
	return e.putFact(ctx, scopeToken, "ledger", struct {
		Hour   int64 `json:"hour"`
		Scan   int   `json:"scan"`
		Demand int   `json:"demand"`
	}{l.Hour, l.Scan, l.Demand})
}

// backoff is the quota stop shared by both Reconcilers.
type backoff struct {
	RetryAt time.Time `json:"retry_at"`
	N       int       `json:"n"`
}

func (e *Engine) loadBackoff(ctx context.Context) backoff {
	var b backoff
	_, _ = e.getFact(ctx, scopeToken, "backoff", &b)
	return b
}

// blockedUntil returns the recorded RetryAt when it is still in the future.
func (e *Engine) blockedUntil(ctx context.Context) (time.Time, bool) {
	b := e.loadBackoff(ctx)
	return b.RetryAt, b.RetryAt.After(e.now())
}

// noteQuota records a quota error: the provider's own retry time when it gave one, else an exponential
// delay of 30 s x 2^n capped at 15 min with +-20% jitter. It returns the time recorded.
func (e *Engine) noteQuota(ctx context.Context, stated time.Time) time.Time {
	b := e.loadBackoff(ctx)
	now := e.now()
	retry := stated.UTC()
	if !retry.After(now) {
		d := backoffMin
		for i := 0; i < b.N && d < backoffMax; i++ {
			d *= 2
		}
		d = min(d, backoffMax)
		jitter := 0.8 + 0.4*e.Jitter() // 0.8 .. 1.2
		retry = now.Add(time.Duration(float64(d) * jitter))
	}
	b.N++
	b.RetryAt = retry
	_ = e.putFact(ctx, scopeToken, "backoff", b)
	return retry
}

// noteOK clears the failure count after a pass that made requests without a quota error.
func (e *Engine) noteOK(ctx context.Context) {
	if b := e.loadBackoff(ctx); b.N > 0 {
		b.N = 0
		_ = e.putFact(ctx, scopeToken, "backoff", b)
	}
}
