package runnerjob

// Purpose: Ledger, the durable list of demands and registered runners (Epic D6, review F8). It lets the
// demand cap count live demands per repository, the finalizer find the registration of an attempt and the
// orphan sweep find the repositories to look at.
// Inputs:  a FactStore (the coordinator's *store.Store implements it) and a clock.
// Outputs: Reserve (cap-checked, idempotent per demand), RecordRunner, Runner, MarkGone and Repos.
// Constraints: rows are provider facts of provider "runnerjob"; a row never holds a credential, only
//          the handle a deregistration needs. A pending demand is live for PendingTTL (the forge
//          honours an unused registration for 60 min), a minted one until the finalizer marks it gone or
//          MintedTTL passes (a runner job lasts at most six hours). Reserve is serialised by a mutex, so
//          two demands of one repository cannot both take the last slot; this holds for one coordinator
//          process, which is the supported shape (the intake store is a single writer).

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Row states.
const (
	statePending = "pending"
	stateMinted  = "minted"
	stateDone    = "done"
	stateGone    = "gone"
)

// Lifetimes of a live demand.
const (
	PendingTTL = 70 * time.Minute
	MintedTTL  = 6*time.Hour + 10*time.Minute
)

const (
	factProvider = "runnerjob"
	indexScope   = "_index"
)

// FactStore is the part of *store.Store the ledger uses.
type FactStore interface {
	ProviderFactPut(ctx context.Context, f store.ProviderFact) (bool, error)
	ProviderFactGet(ctx context.Context, provider, scope, key string) (store.ProviderFact, bool, error)
	ProviderFactList(ctx context.Context, provider, scope, prefix string) ([]store.ProviderFact, error)
}

// Runner is the ledger row of one registration.
type Runner struct {
	Attempt  string    `json:"attempt"`
	Provider string    `json:"provider"`
	Repo     string    `json:"repo"`
	Demand   string    `json:"demand"`
	Handle   string    `json:"handle"`
	State    string    `json:"state"`
	At       time.Time `json:"at"`
}

type demandRow struct {
	State string    `json:"state"`
	At    time.Time `json:"at"`
}

// RepoKey is one repository the sweep must look at.
type RepoKey struct{ Provider, Repo string }

// Ledger keeps the rows.
type Ledger struct {
	fs  FactStore
	now func() time.Time
	mu  sync.Mutex
}

// NewLedger returns a ledger over fs; a nil clock is time.Now.
func NewLedger(fs FactStore, now func() time.Time) *Ledger {
	if now == nil {
		now = time.Now
	}
	return &Ledger{fs: fs, now: now}
}

func scopeOf(repo string) string { return strings.ToLower(repo) }

func demandKey(provider, id string) string { return "demand:" + provider + ":" + id }

func (l *Ledger) put(ctx context.Context, scope, key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	kept, err := l.fs.ProviderFactPut(ctx, store.ProviderFact{Provider: factProvider, Scope: scope, Key: key, Value: raw, ObservedAt: l.now()})
	if err == nil && !kept { // the store keeps the newer observation: a clock that went back must not lose a row silently
		err = errors.New("runnerjob: the ledger holds a newer row than this clock")
	}
	return err
}

func live(r demandRow, now time.Time) bool {
	switch r.State {
	case statePending:
		return now.Sub(r.At) < PendingTTL
	case stateMinted:
		return now.Sub(r.At) < MintedTTL
	}
	return false
}

// Reserve records a demand as pending unless the repository already has max live demands. A demand that
// is already live is accepted again without counting twice (the forge re-offers a queued job). It
// reports false when the cap refuses the demand.
func (l *Ledger) Reserve(ctx context.Context, provider, repo, demand string, max int) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	rows, err := l.fs.ProviderFactList(ctx, factProvider, scopeOf(repo), "demand:")
	if err != nil {
		return false, err
	}
	now, n, key := l.now(), 0, demandKey(provider, demand)
	for _, f := range rows {
		var r demandRow
		if json.Unmarshal(f.Value, &r) != nil || !live(r, now) {
			continue
		}
		if f.Key == key {
			return true, nil
		}
		n++
	}
	if n >= max {
		return false, nil
	}
	if err = l.put(ctx, scopeOf(repo), key, demandRow{State: statePending, At: now}); err != nil {
		return false, err
	}
	return true, l.put(ctx, indexScope, "repo:"+provider+":"+scopeOf(repo), RepoKey{Provider: provider, Repo: repo})
}

// RecordRunner stores the registration of an attempt and marks its demand minted.
func (l *Ledger) RecordRunner(ctx context.Context, r Runner) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	r.State, r.At = stateMinted, l.now()
	if err := l.put(ctx, scopeOf(r.Repo), "runner:"+r.Attempt, r); err != nil {
		return err
	}
	if err := l.put(ctx, scopeOf(r.Repo), demandKey(r.Provider, r.Demand), demandRow{State: stateMinted, At: r.At}); err != nil {
		return err
	}
	return l.put(ctx, indexScope, "repo:"+r.Provider+":"+scopeOf(r.Repo), RepoKey{Provider: r.Provider, Repo: r.Repo})
}

// Runner returns the registration of an attempt, searching the repositories the ledger knows.
func (l *Ledger) Runner(ctx context.Context, attempt string) (Runner, bool, error) {
	repos, err := l.Repos(ctx)
	if err != nil {
		return Runner{}, false, err
	}
	for _, k := range repos {
		f, ok, err := l.fs.ProviderFactGet(ctx, factProvider, scopeOf(k.Repo), "runner:"+attempt)
		if err != nil {
			return Runner{}, false, err
		}
		var r Runner
		if ok && json.Unmarshal(f.Value, &r) == nil {
			return r, true, nil
		}
	}
	return Runner{}, false, nil
}

// MarkGone records that the registration was removed and proven absent, and closes its demand.
func (l *Ledger) MarkGone(ctx context.Context, r Runner) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	r.State, r.At = stateGone, now
	if err := l.put(ctx, scopeOf(r.Repo), "runner:"+r.Attempt, r); err != nil {
		return err
	}
	return l.put(ctx, scopeOf(r.Repo), demandKey(r.Provider, r.Demand), demandRow{State: stateDone, At: now})
}

// Repos lists the repositories that ever held a demand.
func (l *Ledger) Repos(ctx context.Context) ([]RepoKey, error) {
	rows, err := l.fs.ProviderFactList(ctx, factProvider, indexScope, "repo:")
	if err != nil {
		return nil, err
	}
	var out []RepoKey
	for _, f := range rows {
		var k RepoKey
		if json.Unmarshal(f.Value, &k) == nil && k.Repo != "" {
			out = append(out, k)
		}
	}
	return out, nil
}

// Open lists the runner rows of a repository that are not gone, for the sweep.
func (l *Ledger) Open(ctx context.Context, repo string) ([]Runner, error) {
	rows, err := l.fs.ProviderFactList(ctx, factProvider, scopeOf(repo), "runner:")
	if err != nil {
		return nil, err
	}
	var out []Runner
	for _, f := range rows {
		var r Runner
		if json.Unmarshal(f.Value, &r) == nil && r.State == stateMinted {
			out = append(out, r)
		}
	}
	return out, nil
}
