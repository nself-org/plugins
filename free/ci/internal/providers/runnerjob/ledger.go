package runnerjob

// Purpose: Ledger, the durable list of demands and registered runners (Epic D6, review F8). It lets the
// demand cap count live demands per repository, the finalizer find the registration of an attempt and the
// orphan sweep find the repositories to look at.
// Inputs:  a FactStore (the coordinator's *store.Store implements it) and a clock.
// Outputs: Reserve (cap-checked, idempotent per demand), RecordRunner, Release, Runner, MarkGone, Open, Repos.
// Constraints: one provider fact per repository (provider "runnerjob", key "state") holds a small document
//          of the demands and runners that are live or ended in the last hour; every write drops the
//          rest, so the store does not grow with the number of jobs ever run. A row never holds a
//          credential, only the handle a deregistration needs. A pending demand is live for PendingTTL
//          (the forge honours an unused registration for 60 min), a minted one until the finalizer marks
//          it gone or MintedTTL passes (a runner job lasts at most six hours). Every read-modify-write is
//          serialised by one mutex, so two demands of one repository cannot both take the last slot; this
//          holds for one coordinator process, which is the supported shape (the intake store is a single
//          writer).

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

// repoDoc is the state of one repository.
type repoDoc struct {
	Demands map[string]demandRow `json:"demands"`
	Runners map[string]Runner    `json:"runners"`
}

// keepEnded is how long an ended demand or runner row stays in the document.
const keepEnded = time.Hour

// RepoKey is one repository the sweep must look at.
type RepoKey struct{ Provider, Repo string }

// Ledger keeps the rows.
type Ledger struct {
	fs    FactStore
	now   func() time.Time
	mu    sync.Mutex
	where map[string]string // attempt -> repo, for the attempts this process recorded (a restart falls back to the scan)
}

// NewLedger returns a ledger over fs; a nil clock is time.Now.
func NewLedger(fs FactStore, now func() time.Time) *Ledger {
	if now == nil {
		now = time.Now
	}
	return &Ledger{fs: fs, now: now, where: map[string]string{}}
}

func scopeOf(repo string) string { return strings.ToLower(repo) }

func demandKey(provider, id string) string { return provider + ":" + id }

func live(r demandRow, now time.Time) bool {
	switch r.State {
	case statePending:
		return now.Sub(r.At) < PendingTTL
	case stateMinted:
		return now.Sub(r.At) < MintedTTL
	}
	return false
}

// load reads the document of repo; a repository with none yet has an empty one.
func (l *Ledger) load(ctx context.Context, repo string) (repoDoc, error) {
	d := repoDoc{Demands: map[string]demandRow{}, Runners: map[string]Runner{}}
	f, ok, err := l.fs.ProviderFactGet(ctx, factProvider, scopeOf(repo), "state")
	if err != nil || !ok {
		return d, err
	}
	if err = json.Unmarshal(f.Value, &d); err != nil {
		return repoDoc{}, err
	}
	if d.Demands == nil {
		d.Demands = map[string]demandRow{}
	}
	if d.Runners == nil {
		d.Runners = map[string]Runner{}
	}
	return d, nil
}

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

// save drops what ended long ago, writes the document and records the repository in the index.
func (l *Ledger) save(ctx context.Context, provider, repo string, d repoDoc) error {
	now := l.now()
	for k, r := range d.Demands {
		if !live(r, now) && now.Sub(r.At) > keepEnded {
			delete(d.Demands, k)
		}
	}
	for k, r := range d.Runners {
		if r.State != stateMinted && now.Sub(r.At) > keepEnded {
			delete(d.Runners, k)
		}
	}
	if err := l.put(ctx, indexScope, "repo:"+provider+":"+scopeOf(repo), RepoKey{Provider: provider, Repo: repo}); err != nil {
		return err // before the state: a demand is never recorded for a repository the sweep cannot find
	}
	return l.put(ctx, scopeOf(repo), "state", d)
}

// Reserve records a demand as pending unless the repository already has max live demands. A demand that
// is already live is accepted again without counting twice (the forge re-offers a queued job). It
// reports false when the cap refuses the demand.
func (l *Ledger) Reserve(ctx context.Context, provider, repo, demand string, max int) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	d, err := l.load(ctx, repo)
	if err != nil {
		return false, err
	}
	now, key, n := l.now(), demandKey(provider, demand), 0
	for k, r := range d.Demands {
		if !live(r, now) {
			continue
		}
		if k == key {
			return true, nil
		}
		n++
	}
	if n >= max {
		return false, nil
	}
	d.Demands[key] = demandRow{State: statePending, At: now}
	if err = l.save(ctx, provider, repo, d); err != nil {
		return false, err
	}
	return true, nil
}

// RecordRunner stores the registration of an attempt and marks its demand minted.
func (l *Ledger) RecordRunner(ctx context.Context, r Runner) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	d, err := l.load(ctx, r.Repo)
	if err != nil {
		return err
	}
	r.State, r.At = stateMinted, l.now()
	d.Runners[r.Attempt] = r
	d.Demands[demandKey(r.Provider, r.Demand)] = demandRow{State: stateMinted, At: r.At}
	l.where[r.Attempt] = r.Repo
	return l.save(ctx, r.Provider, r.Repo, d)
}

// Release ends the registration of an attempt that is about to be minted again: the runner row is gone
// but the demand keeps its slot (as pending), so another demand cannot take it in between.
func (l *Ledger) Release(ctx context.Context, r Runner) error {
	return l.end(ctx, r, statePending)
}

// MarkGone records that the registration was removed and proven absent, and closes its demand.
func (l *Ledger) MarkGone(ctx context.Context, r Runner) error {
	return l.end(ctx, r, stateDone)
}

func (l *Ledger) end(ctx context.Context, r Runner, demandState string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	d, err := l.load(ctx, r.Repo)
	if err != nil {
		return err
	}
	now := l.now()
	r.State, r.At = stateGone, now
	d.Runners[r.Attempt] = r
	delete(l.where, r.Attempt)
	if !heldByAnother(d, r) { // a retried attempt may hold the same demand: ending the old one must not free the new one's slot
		d.Demands[demandKey(r.Provider, r.Demand)] = demandRow{State: demandState, At: now}
	}
	return l.save(ctx, r.Provider, r.Repo, d)
}

// heldByAnother reports whether a minted runner of another attempt carries r's demand.
func heldByAnother(d repoDoc, r Runner) bool {
	for _, o := range d.Runners {
		if o.State == stateMinted && o.Attempt != r.Attempt && o.Provider == r.Provider && o.Demand == r.Demand {
			return true
		}
	}
	return false
}

// Runner returns the registration of an attempt, searching the repositories the ledger knows.
func (l *Ledger) Runner(ctx context.Context, attempt string) (Runner, bool, error) {
	l.mu.Lock()
	repo, cached := l.where[attempt]
	l.mu.Unlock()
	if cached { // the common answer after a mint in this process: one document, not every repository
		if d, err := l.docOf(ctx, repo); err == nil {
			if r, ok := d.Runners[attempt]; ok {
				return r, true, nil
			}
		}
	}
	repos, err := l.Repos(ctx)
	if err != nil {
		return Runner{}, false, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, k := range repos {
		d, err := l.load(ctx, k.Repo)
		if err != nil {
			return Runner{}, false, err
		}
		if r, ok := d.Runners[attempt]; ok {
			return r, true, nil
		}
	}
	return Runner{}, false, nil
}

// docOf is load under the mutex.
func (l *Ledger) docOf(ctx context.Context, repo string) (repoDoc, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.load(ctx, repo)
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
	l.mu.Lock()
	defer l.mu.Unlock()
	d, err := l.load(ctx, repo)
	if err != nil {
		return nil, err
	}
	var out []Runner
	for _, r := range d.Runners {
		if r.State == stateMinted {
			out = append(out, r)
		}
	}
	return out, nil
}
