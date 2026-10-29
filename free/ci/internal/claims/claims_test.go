package claims

import (
	"fmt"
	"math/rand/v2"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

const runLimit = 30 * time.Minute

func claimAt(host string, at time.Time) Status {
	return Status{Context: "nself-ci/claim", State: "pending", Creator: "station", Description: "claim " + host + " i1", CreatedAt: at}
}

func base(me string, now time.Time, st ...Status) Inputs {
	return Inputs{Now: now, Statuses: st, Me: Me{Host: me, Incarnation: "i1"}, Trusted: []string{"station"},
		RunLimit: runLimit, Hosts: []string{"a", "b", "c"}, Step: time.Minute, RetryMax: 2, RetryGap: time.Minute}
}

func TestNoClaimMeansNoRun(t *testing.T) {
	// the preferred host takes at once, others wait; a host that has no
	// provider time yet never starts on a guess.
	if d := Decide(base("a", t0)); d.Kind != Take {
		t.Fatalf("preferred: %+v", d)
	}
	in := base("b", t0)
	in.FirstSeen = t0
	if d := Decide(in); d.Kind != Wait || !d.Until.Equal(t0.Add(time.Minute)) {
		t.Fatalf("other host: %+v", d)
	}
	in.Now = t0.Add(time.Minute)
	if d := Decide(in); d.Kind != Take {
		t.Fatalf("after step: %+v", d)
	}
	in.Now = time.Time{}
	if d := Decide(in); d.Kind != Wait {
		t.Fatalf("no provider time: %+v", d)
	}
}

func TestClaimOrderingProviderClock(t *testing.T) {
	in := base("b", t0.Add(time.Minute), claimAt("b", t0.Add(10*time.Second)), claimAt("a", t0))
	if d := Decide(in); d.Kind != Lost || d.Winner != "a" {
		t.Fatalf("%+v", d)
	}
	in.Me.Host = "a"
	if d := Decide(in); d.Kind != Take || !d.Holds {
		t.Fatalf("%+v", d)
	}
}

func TestTieLexical(t *testing.T) {
	in := base("b", t0.Add(time.Minute), claimAt("c", t0), claimAt("b", t0), claimAt("a", t0))
	if d := Decide(in); d.Winner != "a" || d.Kind != Lost {
		t.Fatalf("%+v", d)
	}
}

func TestDeadAfterRunLimit(t *testing.T) {
	cl := claimAt("a", t0)
	edge := t0.Add(runLimit + DeadGrace)
	if d := Decide(base("b", edge, cl)); d.Kind != Lost {
		t.Fatalf("still live at the edge: %+v", d)
	}
	in := base("b", edge.Add(time.Second), cl)
	in.FirstSeen = t0
	if d := Decide(in); d.Kind != Take {
		t.Fatalf("dead claim: %+v", d)
	}
	// a run status from the same host keeps the claim alive
	run := Status{Context: "nself-ci", State: "pending", Creator: "station", Description: "running host=a", CreatedAt: edge}
	if d := Decide(base("b", edge.Add(time.Second), cl, run)); d.Kind != Lost {
		t.Fatalf("run status renews: %+v", d)
	}
	// no provider time: never dead
	if d := Decide(base("b", time.Time{}, cl)); d.Kind != Lost {
		t.Fatalf("unknown time: %+v", d)
	}
}

func TestLedgerNeverLocalState(t *testing.T) {
	// a host with no memory of its own run still skips a sha that has a final
	res := Status{Context: "nself-ci", State: "success", Creator: "station", CreatedAt: t0}
	for _, h := range []string{"a", "b", "c"} {
		if d := Decide(base(h, t0.Add(time.Hour), res)); d.Kind != Skip {
			t.Fatalf("%s: %+v", h, d)
		}
	}
}

func TestForgedStatusIgnored(t *testing.T) {
	forgedFinal := Status{Context: "nself-ci", State: "success", Creator: "mallory", CreatedAt: t0}
	forgedClaim := Status{Context: "nself-ci/claim", State: "pending", Creator: "mallory", Description: "claim a i1", CreatedAt: t0}
	if d := Decide(base("a", t0.Add(time.Minute), forgedFinal, forgedClaim)); d.Kind != Take || d.Holds {
		t.Fatalf("%+v", d)
	}
}

func TestFinalPublicationFence(t *testing.T) {
	mine := claimAt("a", t0)
	if !MayPublish(base("a", t0.Add(time.Minute), mine)) {
		t.Fatal("holder must publish")
	}
	if MayPublish(base("b", t0.Add(time.Minute), mine, claimAt("b", t0.Add(time.Second)))) {
		t.Fatal("loser must keep the result as evidence")
	}
	fin := Status{Context: "nself-ci", State: "failure", Creator: "station", CreatedAt: t0.Add(time.Second)}
	if MayPublish(base("a", t0.Add(time.Minute), mine, fin)) {
		t.Fatal("a final already exists")
	}
}

func TestErrorUnderBudgetNotFinal(t *testing.T) {
	e := func(i int) Status {
		return Status{Context: "nself-ci", State: "error", Creator: "station", CreatedAt: t0.Add(time.Duration(i) * time.Hour)}
	}
	if Decide(base("a", t0.Add(3*time.Hour), e(0), e(1))).Kind != Take {
		t.Fatal("two errors with RetryMax 2 is a retry candidate")
	}
	if Decide(base("a", t0.Add(4*time.Hour), e(0), e(1), e(2))).Kind != Skip {
		t.Fatal("RetryMax+1 errors is final")
	}
	if d := Decide(base("a", t0.Add(time.Hour+time.Second), e(0), e(1))); d.Kind != Wait {
		t.Fatalf("inside the retry gap: %+v", d)
	}
	// an old claim from the failed attempt does not block the retry
	if d := Decide(base("a", t0.Add(3*time.Hour), claimAt("b", t0.Add(-time.Minute)), e(0))); d.Kind != Take {
		t.Fatalf("%+v", d)
	}
}

// TestDecideProperty feeds 10 000 generated ledgers with skew, ties, dead
// claims, partial lists and foreign writers and checks the invariants.
func TestDecideProperty(t *testing.T) {
	seed := uint64(20261027)
	r := rand.New(rand.NewPCG(seed, 7))
	hosts := []string{"a", "b", "c", "d"}
	for i := 0; i < 10000; i++ {
		now := t0.Add(time.Duration(r.IntN(3*3600)) * time.Second)
		var st []Status
		for n := r.IntN(8); n > 0; n-- {
			creator := "station"
			if r.IntN(4) == 0 {
				creator = "mallory"
			}
			skew := time.Duration(r.IntN(1200)-600) * time.Second
			at := t0.Add(time.Duration(r.IntN(7200))*time.Second + skew)
			if r.IntN(3) == 0 {
				at = t0 // ties
			}
			if r.IntN(15) == 0 {
				st = append(st, Status{Context: "nself-ci", State: []string{"success", "failure", "error"}[r.IntN(3)], Creator: creator, CreatedAt: at})
				continue
			}
			s := claimAt(hosts[r.IntN(4)], at)
			s.Creator = creator
			st = append(st, s)
		}
		if r.IntN(5) == 0 && len(st) > 0 {
			st = st[:len(st)/2] // partial list
		}
		winners, final := 0, false
		var win string
		for _, h := range hosts {
			in := base(h, now, st...)
			in.Hosts = hosts
			d := Decide(in)
			if d.Kind == Skip {
				final = true
			}
			if d.Kind == Take && d.Holds {
				winners++
				win = h
			}
			if d.Kind == Lost && d.Winner == h {
				t.Fatalf("seed %d case %d: lost to self", seed, i)
			}
		}
		if winners > 1 {
			t.Fatalf("seed %d case %d: %d live winners", seed, i, winners)
		}
		// forged statuses never change the outcome
		var clean []Status
		for _, s := range st {
			if s.Creator == "station" {
				clean = append(clean, s)
			}
		}
		a, b := base("a", now, st...), base("a", now, clean...)
		a.Hosts, b.Hosts = hosts, hosts
		if Decide(a) != Decide(b) {
			t.Fatalf("seed %d case %d: foreign writer changed the outcome", seed, i)
		}
		if !final && winners == 1 && win == "" {
			t.Fatal("unreachable")
		}
	}
}

func ExampleParseClaim() {
	h, i, ok := ParseClaim("claim mm6 inc9")
	fmt.Println(h, i, ok)
	// Output: mm6 inc9 true
}
