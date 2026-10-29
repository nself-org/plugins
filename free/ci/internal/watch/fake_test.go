package watch

// Purpose: the shared test rig: an in-memory provider status ledger with its own clock and partitions,
//          a fixed scanner, a scripted runner and a manual After.
// Constraints: no network, no sleeps, no ports; time is a value the test advances.

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

var t0 = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

func sha(n int) string { return strings.Repeat("a", 39) + strconv.Itoa(n%10) }

type fakeGH struct {
	mu     sync.Mutex
	now    time.Time
	st     map[string][]report.LedgerStatus
	down   map[string]bool
	claims int
	posts  []report.Status
}

func newGH() *fakeGH {
	return &fakeGH{now: t0, st: map[string][]report.LedgerStatus{}, down: map[string]bool{}}
}

func (g *fakeGH) advance(d time.Duration) { g.mu.Lock(); g.now = g.now.Add(d); g.mu.Unlock() }
func (g *fakeGH) clock() time.Time        { g.mu.Lock(); defer g.mu.Unlock(); return g.now }

// on returns the view of host h.
func (g *fakeGH) on(h string) *hostGH { return &hostGH{g: g, host: h} }

// states lists the states posted on ctxName for sha, oldest first.
func (g *fakeGH) states(s, ctxName string) []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []string
	for _, x := range g.st[s] {
		if x.Context == ctxName {
			out = append(out, string(x.State))
		}
	}
	return out
}

type hostGH struct {
	g    *fakeGH
	host string
}

var errPartition = errors.New("partition")

func (h *hostGH) add(s report.Status) error {
	g := h.g
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.down[h.host] {
		return errPartition
	}
	g.st[s.Revision] = append(g.st[s.Revision], report.LedgerStatus{Context: s.ContextName(), State: s.State, Creator: "station", Description: s.Description, CreatedAt: g.now})
	return nil
}

func (h *hostGH) PostClaim(_ context.Context, s report.Status) (time.Time, error) {
	if err := h.add(s); err != nil {
		return time.Time{}, err
	}
	h.g.mu.Lock()
	h.g.claims++
	h.g.mu.Unlock()
	return h.g.clock(), nil
}

func (h *hostGH) Post(_ context.Context, s report.Status) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := h.add(s); err != nil {
		return err
	}
	h.g.mu.Lock()
	h.g.posts = append(h.g.posts, s)
	h.g.mu.Unlock()
	return nil
}

func (h *hostGH) ListStatuses(_ context.Context, _, _, s string) ([]report.LedgerStatus, time.Time, error) {
	g := h.g
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.down[h.host] {
		return nil, time.Time{}, errPartition
	}
	src := g.st[s]
	out := make([]report.LedgerStatus, 0, len(src))
	for i := len(src) - 1; i >= 0; i-- {
		out = append(out, src[i])
	}
	return out, g.now, nil
}

type fixedScan struct {
	heads []Head
	calls int
	err   error
}

func (f *fixedScan) Scan(context.Context, string) (Scan, error) {
	f.calls++
	if f.err != nil {
		return Scan{}, f.err
	}
	return Scan{Heads: append([]Head(nil), f.heads...)}, nil
}

type scriptRunner struct {
	mu      sync.Mutex
	jobs    []Job
	state   func(n int, j Job) report.State
	block   chan struct{} // when set, Run waits for it or for its context
	started chan struct{} // when set, Run signals it on entry
}

func (r *scriptRunner) Run(ctx context.Context, j Job) (Outcome, error) {
	r.mu.Lock()
	r.jobs = append(r.jobs, j)
	n := len(r.jobs)
	r.mu.Unlock()
	if r.started != nil {
		r.started <- struct{}{}
	}
	if r.block != nil {
		select {
		case <-r.block:
		case <-ctx.Done():
			return Outcome{State: report.Error, Description: "cancelled"}, ctx.Err()
		}
	}
	st := report.Success
	if r.state != nil {
		st = r.state(n, j)
	}
	return Outcome{State: st, Description: "gate " + string(st)}, nil
}

func (r *scriptRunner) count() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.jobs) }

func ownerHead(repo string, n int) Head {
	return Head{Repo: repo, SHA: sha(n), Branch: "main", Default: true, Protected: true, Trust: model.TrustClass("owner")}
}

type rig struct {
	gh     *fakeGH
	scan   *fixedScan
	runner *scriptRunner
	w      *Watcher
	alerts []string
}

// newRig is one host "a" watching o/r, ready, unpinned, with the given heads.
func newRig(t *testing.T, heads ...Head) *rig {
	t.Helper()
	r := &rig{gh: newGH(), scan: &fixedScan{heads: heads}, runner: &scriptRunner{}}
	c := Defaults()
	c.Repos, c.Host, c.Hosts, c.Trusted = []string{"o/r"}, "a", []string{"a"}, []string{"station"}
	p := Ports{Scanner: r.scan, Ledger: r.gh.on("a"), Reporter: r.gh.on("a"), Runner: r.runner, Now: r.gh.clock,
		Ready:         func(context.Context) (bool, []string) { return true, nil },
		ResultContext: func(context.Context, string, string, string) string { return report.Context },
		Alert:         func(_ context.Context, h Head, d string) { r.alerts = append(r.alerts, h.SHA+" "+d) }}
	w, err := New(c, p)
	if err != nil {
		t.Fatal(err)
	}
	r.w = w
	return r
}
