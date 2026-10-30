package sim

// Purpose: P7-AUTO-11 Sim A3: three `serve watch` hosts against one fixture repository (no app-pinned
//          check) over an in-memory provider ledger: random pushes and partitions, then healing; a
//          preferred host killed before and after it claims.
// Inputs:  watch.Watcher with fake ports. Outputs: pass or fail.
// Constraints: no sleeps, no ports; time is the provider clock the test advances; the generator is seeded
//          and the seed is printed on failure.

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/claims"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/watch"
)

var wT0 = time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)

const (
	wStep     = time.Minute
	wRunLimit = 30 * time.Minute
)

type wProv struct {
	mu    sync.Mutex
	now   time.Time
	st    map[string][]report.LedgerStatus
	down  map[string]bool
	heads []watch.Head
	runs  map[string]int // host -> attempts started
}

func newWProv() *wProv {
	return &wProv{now: wT0, st: map[string][]report.LedgerStatus{}, down: map[string]bool{}, runs: map[string]int{}}
}

func (p *wProv) clock() time.Time { p.mu.Lock(); defer p.mu.Unlock(); return p.now }

func (p *wProv) push(n int) string {
	s := fmt.Sprintf("%040x", n+1)
	p.mu.Lock()
	p.heads = append(p.heads, watch.Head{SHA: s, Branch: fmt.Sprintf("b%d", n), Protected: true, Trust: model.TrustClass("owner")})
	p.mu.Unlock()
	return s
}

func (p *wProv) finals(s string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, x := range p.st[s] {
		if x.Context == report.Context && (x.State == report.Success || x.State == report.Failure) {
			n++
		}
	}
	return n
}

type wHost struct {
	p  *wProv
	id string
}

var errDown = errors.New("partition")

func (h wHost) add(s report.Status) error {
	h.p.mu.Lock()
	defer h.p.mu.Unlock()
	if h.p.down[h.id] {
		return errDown
	}
	h.p.st[s.Revision] = append(h.p.st[s.Revision], report.LedgerStatus{Context: s.ContextName(), State: s.State, Creator: "station", Description: s.Description, CreatedAt: h.p.now})
	return nil
}

func (h wHost) PostClaim(_ context.Context, s report.Status) (time.Time, error) {
	return h.p.clock(), h.add(s)
}

func (h wHost) Post(_ context.Context, s report.Status) error { return h.add(s) }

func (h wHost) ListStatuses(_ context.Context, _, _, s string) ([]report.LedgerStatus, time.Time, error) {
	h.p.mu.Lock()
	defer h.p.mu.Unlock()
	if h.p.down[h.id] {
		return nil, time.Time{}, errDown
	}
	return append([]report.LedgerStatus(nil), h.p.st[s]...), h.p.now, nil
}

func (h wHost) Scan(context.Context, string) (watch.Scan, error) {
	h.p.mu.Lock()
	defer h.p.mu.Unlock()
	if h.p.down[h.id] {
		return watch.Scan{}, errDown
	}
	return watch.Scan{Heads: append([]watch.Head(nil), h.p.heads...)}, nil
}

// wRunner runs instantly; kill, when set, is called on entry (the host dies after it claimed).
type wRunner struct {
	h    wHost
	kill func()
}

func (r *wRunner) Run(context.Context, watch.Job) (watch.Outcome, error) {
	r.h.p.mu.Lock()
	r.h.p.runs[r.h.id]++
	r.h.p.mu.Unlock()
	if r.kill != nil {
		r.kill()
	}
	return watch.Outcome{State: report.Success, Description: "ok"}, nil
}

func newWatchHost(t *testing.T, p *wProv, id string) (*watch.Watcher, *wRunner) {
	t.Helper()
	h := wHost{p: p, id: id}
	run := &wRunner{h: h}
	c := watch.Defaults()
	c.Repos, c.Host, c.Hosts, c.Trusted, c.Step, c.RunLimit = []string{"o/r"}, id, []string{"a", "b", "c"}, []string{"station"}, wStep, wRunLimit
	w, err := watch.New(c, watch.Ports{Scanner: h, Ledger: h, Reporter: h, Runner: run, Now: p.clock,
		Ready:         func(context.Context) (bool, []string) { return true, nil },
		ResultContext: func(context.Context, string, string, string) string { return report.Context }})
	if err != nil {
		t.Fatal(err)
	}
	return w, run
}

func TestWatchThreeHostsExactlyOneStatus(t *testing.T) {
	seed := uint64(time.Now().UnixNano())
	rng := rand.New(rand.NewPCG(seed, 11))
	p := newWProv()
	ws := map[string]*watch.Watcher{}
	for _, id := range []string{"a", "b", "c"} {
		ws[id], _ = newWatchHost(t, p, id)
	}
	var pushed []string
	dups := 0
	pass := func() {
		for _, id := range []string{"a", "b", "c"} {
			dups += ws[id].Pass(context.Background()).Duplicates
		}
		p.mu.Lock()
		p.now = p.now.Add(30 * time.Second)
		p.mu.Unlock()
	}
	for i := 0; i < 120; i++ { // pushes and partitions
		if rng.IntN(6) == 0 {
			pushed = append(pushed, p.push(len(pushed)))
		}
		if rng.IntN(5) == 0 {
			p.mu.Lock()
			id := []string{"a", "b", "c"}[rng.IntN(3)]
			p.down[id] = !p.down[id]
			p.mu.Unlock()
		}
		pass()
	}
	p.mu.Lock()
	p.down = map[string]bool{} // the partitions heal
	p.mu.Unlock()
	for i := 0; i < 200; i++ {
		pass()
	}
	total := 0
	for _, s := range pushed {
		n := p.finals(s)
		if n == 0 {
			t.Fatalf("seed %d: commit %s has no final result", seed, s[:12])
		}
		total += n - 1
	}
	if total != dups {
		t.Fatalf("seed %d: %d duplicate finals, %d reported", seed, total, dups)
	}
	if len(pushed) == 0 {
		t.Logf("seed %d pushed nothing", seed)
	}
}

func TestWatchKillPreferredTakeover(t *testing.T) {
	// killed before it claims: b starts after step_sec, c sees the final
	p := newWProv()
	s := p.push(0)
	wb, _ := newWatchHost(t, p, "b")
	wc, _ := newWatchHost(t, p, "c")
	start := p.clock()
	for p.finals(s) == 0 && p.clock().Sub(start) < 10*time.Minute {
		wb.Pass(context.Background())
		wc.Pass(context.Background())
		p.mu.Lock()
		p.now = p.now.Add(10 * time.Second)
		p.mu.Unlock()
	}
	if p.finals(s) != 1 || p.runs["b"] != 1 || p.runs["c"] != 0 {
		t.Fatalf("finals %d, runs %v", p.finals(s), p.runs)
	}
	if took := p.st[s][0].CreatedAt.Sub(start); took < wStep {
		t.Fatalf("b claimed after %v, before step %v", took, wStep)
	}

	// killed after it claimed: nobody finishes until the claim is dead
	p = newWProv()
	s = p.push(0)
	wa, ra := newWatchHost(t, p, "a")
	ra.kill = func() { p.mu.Lock(); p.down["a"] = true; p.mu.Unlock() }
	wa.Pass(context.Background())
	if p.finals(s) != 0 || p.runs["a"] != 1 {
		t.Fatalf("a died after its claim: finals %d", p.finals(s))
	}
	wb, _ = newWatchHost(t, p, "b")
	wc, _ = newWatchHost(t, p, "c")
	dead := wT0.Add(wRunLimit + claims.DeadGrace)
	for p.clock().Before(dead.Add(5 * time.Minute)) {
		if p.clock().Before(dead) && p.finals(s) != 0 {
			t.Fatalf("a final at %v while a's claim was live", p.clock().Sub(wT0))
		}
		wb.Pass(context.Background())
		wc.Pass(context.Background())
		p.mu.Lock()
		p.now = p.now.Add(time.Minute)
		p.mu.Unlock()
	}
	if p.finals(s) != 1 {
		t.Fatalf("after the claim died: finals %d, runs %v", p.finals(s), p.runs)
	}
}
