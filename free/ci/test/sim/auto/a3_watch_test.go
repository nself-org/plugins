package auto

// Purpose: A3 three watch hosts, one fixture repository: every pushed commit gets a final result and
// the duplicates the hosts report match the duplicates the ledger holds.
// Inputs: watch.Watcher over an in-memory provider ledger; seeded pushes and partitions, then healing.
// Outputs: pass or fail. Constraints: time is the ledger clock the test advances; the seed is printed.

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/watch"
)

var errPartition = errors.New("partition")

type ledger struct {
	mu    sync.Mutex
	now   time.Time
	st    map[string][]report.LedgerStatus
	down  map[string]bool
	heads []watch.Head
}

func (l *ledger) clock() time.Time { l.mu.Lock(); defer l.mu.Unlock(); return l.now }

func (l *ledger) push(n int) string {
	sha := fmt.Sprintf("%040x", n+1)
	l.mu.Lock()
	l.heads = append(l.heads, watch.Head{SHA: sha, Branch: fmt.Sprintf("b%d", n), Protected: true, Trust: model.TrustClass("owner")})
	l.mu.Unlock()
	return sha
}

func (l *ledger) finals(sha string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, x := range l.st[sha] {
		if x.Context == report.Context && (x.State == report.Success || x.State == report.Failure) {
			n++
		}
	}
	return n
}

type host struct {
	l  *ledger
	id string
}

func (h host) add(s report.Status) error {
	h.l.mu.Lock()
	defer h.l.mu.Unlock()
	if h.l.down[h.id] {
		return errPartition
	}
	h.l.st[s.Revision] = append(h.l.st[s.Revision], report.LedgerStatus{Context: s.ContextName(), State: s.State, Creator: "station", Description: s.Description, CreatedAt: h.l.now})
	return nil
}

func (h host) PostClaim(_ context.Context, s report.Status) (time.Time, error) {
	return h.l.clock(), h.add(s)
}
func (h host) Post(_ context.Context, s report.Status) error { return h.add(s) }

func (h host) ListStatuses(_ context.Context, _, _, sha string) ([]report.LedgerStatus, time.Time, error) {
	h.l.mu.Lock()
	defer h.l.mu.Unlock()
	if h.l.down[h.id] {
		return nil, time.Time{}, errPartition
	}
	return append([]report.LedgerStatus(nil), h.l.st[sha]...), h.l.now, nil
}

func (h host) Scan(context.Context, string) (watch.Scan, error) {
	h.l.mu.Lock()
	defer h.l.mu.Unlock()
	if h.l.down[h.id] {
		return watch.Scan{}, errPartition
	}
	return watch.Scan{Heads: append([]watch.Head(nil), h.l.heads...)}, nil
}

type instant struct{}

func (instant) Run(context.Context, watch.Job) (watch.Outcome, error) {
	return watch.Outcome{State: report.Success, Description: "ok"}, nil
}

func newHost(t *testing.T, l *ledger, id string) *watch.Watcher {
	t.Helper()
	h := host{l: l, id: id}
	c := watch.Defaults()
	c.Repos, c.Host, c.Hosts, c.Trusted, c.Step, c.RunLimit = []string{"o/r"}, id, []string{"a", "b", "c"}, []string{"station"}, time.Minute, 30*time.Minute
	w, err := watch.New(c, watch.Ports{Scanner: h, Ledger: h, Reporter: h, Runner: instant{}, Now: l.clock,
		Ready:         func(context.Context) (bool, []string) { return true, nil },
		ResultContext: func(context.Context, string, string, string) string { return report.Context }})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestConformanceA3Watch(t *testing.T) {
	seed := uint64(time.Now().UnixNano())
	rng := rand.New(rand.NewPCG(seed, 3))
	l := &ledger{now: time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC), st: map[string][]report.LedgerStatus{}, down: map[string]bool{}}
	ids := []string{"a", "b", "c"}
	ws := map[string]*watch.Watcher{}
	for _, id := range ids {
		ws[id] = newHost(t, l, id)
	}
	var pushed []string
	dups := 0
	pass := func() {
		for _, id := range ids {
			dups += ws[id].Pass(context.Background()).Duplicates
		}
		l.mu.Lock()
		l.now = l.now.Add(30 * time.Second)
		l.mu.Unlock()
	}
	for i := 0; i < 120; i++ {
		if rng.IntN(6) == 0 {
			pushed = append(pushed, l.push(len(pushed)))
		}
		if rng.IntN(5) == 0 {
			l.mu.Lock()
			id := ids[rng.IntN(3)]
			l.down[id] = !l.down[id]
			l.mu.Unlock()
		}
		pass()
	}
	l.mu.Lock()
	l.down = map[string]bool{}
	l.mu.Unlock()
	for i := 0; i < 200; i++ {
		pass()
	}
	total := 0
	for _, sha := range pushed {
		n := l.finals(sha)
		if n == 0 {
			t.Fatalf("seed %d: commit %s has no final result", seed, sha[:12])
		}
		total += n - 1
	}
	if total != dups {
		t.Fatalf("seed %d: %d duplicate finals in the ledger, %d reported", seed, total, dups)
	}
}
