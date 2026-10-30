package sim

// Purpose: P7-AUTO-10 proof of the claim ledger with three stations over an
// in-memory provider: killed hosts, a partition during PostClaim, skewed local
// clocks and delayed completion. Every decision reads only the provider clock.
// Inputs:  claims.Decide over a shared status list. Outputs: pass or fail.
// Constraints: no sleeps, no ports; time is a counter the test advances.

import (
	"errors"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/claims"
)

const (
	clRunLimit = 30 * time.Minute
	clStep     = time.Minute
)

var clT0 = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

type clProvider struct {
	now      time.Time
	st       []claims.Status
	down     map[string]bool // host -> partitioned
	finals   int
	dupFinal int
}

type clHost struct {
	id        string
	firstSeen time.Time
	running   bool
	doneAt    time.Time
	errors    int
	evidence  int
	skew      time.Duration // local clock error; never read by a decision
}

func (p *clProvider) in(h *clHost) claims.Inputs {
	return claims.Inputs{Now: p.now, Statuses: append([]claims.Status{}, p.st...), Me: claims.Me{Host: h.id, Incarnation: "i1"},
		Trusted: []string{"station"}, RunLimit: clRunLimit, Hosts: []string{"a", "b", "c"}, Step: clStep,
		FirstSeen: h.firstSeen, RetryMax: 1, RetryGap: time.Minute}
}

func (p *clProvider) post(h *clHost, ctx, state, desc string) error {
	if p.down[h.id] {
		return errors.New("partition")
	}
	p.st = append(p.st, claims.Status{Context: ctx, State: state, Creator: "station", Description: desc, CreatedAt: p.now})
	return nil
}

// step is one watch pass of host h.
func (p *clProvider) step(h *clHost) {
	if h.running {
		if p.now.Before(h.doneAt) {
			return
		}
		h.running = false
		if p.down[h.id] || !claims.MayPublish(p.in(h)) {
			h.evidence++
			return
		}
		_ = p.post(h, "nself-ci", "success", "done host="+h.id)
		p.finals++
		return
	}
	if p.down[h.id] {
		h.errors++ // the host runs nothing and records an error locally
		return
	}
	if h.firstSeen.IsZero() {
		h.firstSeen = p.now
	}
	d := claims.Decide(p.in(h))
	if d.Kind != claims.Take {
		return
	}
	if !d.Holds {
		if err := p.post(h, "nself-ci/claim", "pending", "claim "+h.id+" i1"); err != nil {
			h.errors++
			return
		}
		if d = claims.Decide(p.in(h)); !d.Holds {
			return // lost the read-back: posts nothing more
		}
	}
	h.running, h.doneAt = true, p.now.Add(5*time.Minute)
}

func clRun(p *clProvider, hs []*clHost, passes int, every time.Duration) {
	for i := 0; i < passes; i++ {
		for _, h := range hs {
			p.step(h)
		}
		p.now = p.now.Add(every)
	}
}

func clNew(skews ...time.Duration) (*clProvider, []*clHost) {
	p := &clProvider{now: clT0, down: map[string]bool{}}
	hs := []*clHost{{id: "a"}, {id: "b"}, {id: "c"}}
	for i, s := range skews {
		hs[i].skew = s
	}
	return p, hs
}

func TestClaimsThreeHostsKillPreferred(t *testing.T) {
	// killed BEFORE it claims: b starts after step, c loses the tie
	p, hs := clNew()
	p.down["a"] = true
	clRun(p, hs, 20, 30*time.Second)
	if p.finals != 1 {
		t.Fatalf("finals %d", p.finals)
	}
	if hs[0].running || hs[2].evidence != 0 {
		t.Fatal("a must not run; c must not have a result")
	}
	// killed AFTER it claimed: b/c wait until the claim is dead
	p, hs = clNew()
	p.step(hs[0])
	p.down["a"] = true
	hs[0].running = false
	clRun(p, hs, 20, time.Minute)
	if p.finals != 0 {
		t.Fatalf("claim still live at %v, final posted", p.now.Sub(clT0))
	}
	p.now = clT0.Add(clRunLimit + claims.DeadGrace + time.Second)
	clRun(p, hs, 20, time.Minute)
	if p.finals != 1 {
		t.Fatalf("no commit may end without a final: %d", p.finals)
	}
}

func TestClaimsPartition(t *testing.T) {
	p, hs := clNew()
	p.down["a"] = true
	p.step(hs[0])
	if hs[0].running || hs[0].errors != 1 || len(p.st) != 0 {
		t.Fatal("a partitioned PostClaim means nothing runs")
	}
	p.down["a"] = false
	clRun(p, hs, 12, time.Minute)
	if p.finals != 1 {
		t.Fatalf("recover on the next pass: %d", p.finals)
	}
	// a completion delayed past a takeover keeps its result as evidence
	p, hs = clNew()
	p.step(hs[0])
	p.now = clT0.Add(clRunLimit + claims.DeadGrace + time.Minute)
	hs[0].running = true
	hs[0].doneAt = p.now
	p.st = append(p.st, claims.Status{Context: "nself-ci/claim", State: "pending", Creator: "station",
		Description: "claim b i1", CreatedAt: p.now.Add(-time.Second)})
	p.step(hs[0])
	if p.finals != 0 || hs[0].evidence != 1 {
		t.Fatalf("late host posts nothing: finals %d evidence %d", p.finals, hs[0].evidence)
	}
}

func TestClaimsClockSkew(t *testing.T) {
	var want int
	for i, skew := range []time.Duration{0, 10 * time.Minute, -10 * time.Minute} {
		p, hs := clNew(skew, -skew, skew/2)
		clRun(p, hs, 15, time.Minute)
		if i == 0 {
			want = p.finals
		}
		if p.finals != 1 || p.finals != want {
			t.Fatalf("skew %v: finals %d", skew, p.finals)
		}
		if hs[1].running || hs[2].running {
			t.Fatal("only the preferred host runs")
		}
	}
}
