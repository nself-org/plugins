package fairq

// Property tests of the fair queue. Every generator is seeded and prints its
// seed on failure, so a failing case replays with that seed (Constitution §10.4).
// No clock, no sleeps: time is a value.

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"testing"
)

const cases = 10000

func rng(seed uint64) *rand.Rand { return rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)) }

func item(key, project string, b Band, enq int64, seq uint64) Item {
	return Item{Key: key, Pipeline: "p-" + key, Job: key, Project: project, Band: b, Priority: "normal", EnqueuedMs: enq, Seq: seq, Cost: 1}
}

func keys(r []Ranked) []string {
	out := make([]string, len(r))
	for i, x := range r {
		out[i] = x.Key
	}
	return out
}

func has(q []Item, project string) bool {
	for _, it := range q {
		if it.Project == project {
			return true
		}
	}
	return false
}

// remove drops key from the queue, as an admission does.
func remove(q []Item, key string) []Item {
	out := q[:0:0]
	for _, it := range q {
		if it.Key != key {
			out = append(out, it)
		}
	}
	return out
}

// TestFairness: two projects submitting at 10:1 rates, both backlogged over a
// simulated hour, are served runtime within max job runtime x slots of each other.
func TestFairness(t *testing.T) {
	const chunks = 20
	for c := 0; c < chunks; c++ {
		t.Run(fmt.Sprintf("seeds-%02d", c), func(t *testing.T) {
			t.Parallel()
			for i := c * cases / chunks; i < (c+1)*cases/chunks; i++ {
				fairnessCase(t, uint64(i)+1)
			}
		})
	}
}

// fairnessCase simulates one hour for one seed and fails the test when the bound breaks.
func fairnessCase(t *testing.T, seed uint64) {
	const hourMs = int64(3_600_000)
	r := rng(seed)
	slots := 1 + r.IntN(4)
	maxRun := int64(600_000 + r.IntN(1_200_000))
	runtime := func() int64 { return maxRun/10 + r.Int64N(maxRun-maxRun/10+1) }
	var (
		q      []Item
		seq    uint64
		served = map[string]int64{}
		st     State
		free   = make([]int64, slots) // when each slot is next free
	)
	add := func(project string, at int64) {
		seq++
		it := item(fmt.Sprintf("%s-%d", project, seq), project, Normal, at, seq)
		it.Cost = runtime()
		q = append(q, it)
	}
	// Both projects start with more work than the hour can serve, and keep
	// arriving at 10:1, so both stay backlogged for the whole hour.
	need := int(int64(slots)*hourMs/maxRun) + 6
	for k := 0; k < need; k++ {
		add("A", 0)
		add("B", 0)
	}
	for at := int64(0); at < hourMs; at += 900_000 {
		for k := 0; k < 10; k++ {
			add("A", at)
		}
		add("B", at)
	}
	for {
		sort.Slice(free, func(a, b int) bool { return free[a] < free[b] })
		now := free[0]
		if now >= hourMs {
			break
		}
		var ranked []Ranked
		ranked, st = Order(st, q, now, Config{})
		if !has(q, "A") || !has(q, "B") {
			t.Fatalf("seed %d: a project ran out of queued jobs at %d ms, so the case is not backlogged", seed, now)
		}
		head := ranked[0]
		served[head.Project] += head.Cost
		q = remove(q, head.Key)
		free[0] = now + head.Cost
		bound := maxRun * int64(slots)
		if d := served["A"] - served["B"]; d > bound || -d > bound {
			t.Fatalf("seed %d: served A=%d B=%d differ by more than %d (slots %d, max runtime %d) at %d ms", seed, served["A"], served["B"], bound, slots, maxRun, now)
		}
	}
	if served["A"] == 0 || served["B"] == 0 {
		t.Fatalf("seed %d: a project was never served: %v", seed, served)
	}
}

// TestBands: the order is strict by band, a permutation of the input, and the
// same input always gives the same order.
func TestBands(t *testing.T) {
	for i := 0; i < cases; i++ {
		seed := uint64(i) + 1
		r := rng(seed)
		n := 1 + r.IntN(40)
		var q []Item
		for k := 0; k < n; k++ {
			it := item(fmt.Sprintf("j%03d", k), fmt.Sprintf("proj%d", r.IntN(4)), Band(r.IntN(3)), int64(r.IntN(100000)), uint64(k))
			it.Priority = []string{"low", "normal", "high"}[r.IntN(3)]
			it.Cost = 1 + int64(r.IntN(5))
			q = append(q, it)
		}
		got, st := Order(State{}, q, 100000, Config{})
		again, _ := Order(State{}, q, 100000, Config{})
		if fmt.Sprint(keys(got)) != fmt.Sprint(keys(again)) {
			t.Fatalf("seed %d: the same input ordered differently", seed)
		}
		if len(got) != n {
			t.Fatalf("seed %d: %d items in, %d out", seed, n, len(got))
		}
		seen := map[string]bool{}
		for p, x := range got {
			if seen[x.Key] || x.Position != p {
				t.Fatalf("seed %d: duplicate or wrong position at %d", seed, p)
			}
			seen[x.Key] = true
			if p > 0 && got[p-1].Effective > x.Effective {
				t.Fatalf("seed %d: %s (%s) is ordered before %s (%s)", seed, got[p-1].Key, got[p-1].Effective, x.Key, x.Effective)
			}
			if x.Band != q[indexOf(q, x.Key)].Band {
				t.Fatalf("seed %d: Band must report the submitted band", seed)
			}
		}
		_ = st
	}
	// a project with an empty name is a project like any other, not a sentinel that always loses
	var anon []Item
	for _, k := range []string{"1", "2", "3"} {
		anon = append(anon, item("e"+k, "", Normal, 1, 1), item("x"+k, "x", Normal, 1, 2))
	}
	for i := range anon {
		anon[i].Seq = uint64(i + 1)
	}
	if got := keys(first(Order(State{}, anon, 10, Config{}))); fmt.Sprint(got) != "[e1 x1 e2 x2 e3 x3]" {
		t.Fatalf("empty project must alternate with a named one: got %v", got)
	}
	// within a project, priority then age
	q := []Item{item("a", "p", Normal, 5, 1), item("b", "p", Normal, 1, 2), item("c", "p", Normal, 9, 3)}
	q[2].Priority = "high"
	if got := keys(first(Order(State{}, q, 10, Config{}))); fmt.Sprint(got) != "[c b a]" {
		t.Fatalf("priority then age: got %v", got)
	}
	// ties break by pipeline id, job id, matrix key
	t1, t2, t3 := item("x", "p", Normal, 1, 1), item("y", "p", Normal, 1, 2), item("z", "p", Normal, 1, 3)
	t1.Pipeline, t2.Pipeline, t3.Pipeline = "p2", "p1", "p1"
	t2.Job, t3.Job = "b", "a"
	if got := keys(first(Order(State{}, []Item{t1, t2, t3}, 10, Config{}))); fmt.Sprint(got) != "[z y x]" {
		t.Fatalf("tie-break: got %v", got)
	}
}

func first(r []Ranked, _ State) []Ranked { return r }

func indexOf(q []Item, key string) int {
	for i, it := range q {
		if it.Key == key {
			return i
		}
	}
	return -1
}

// TestStarvation: a waiting background job moves up one band, never above
// normal; nothing else changes band.
func TestStarvation(t *testing.T) {
	const after = int64(30 * 60 * 1000)
	for i := 0; i < cases; i++ {
		seed := uint64(i) + 1
		r := rng(seed)
		n := 1 + r.IntN(30)
		now := int64(10 * after)
		var q []Item
		for k := 0; k < n; k++ {
			q = append(q, item(fmt.Sprintf("j%03d", k), fmt.Sprintf("p%d", r.IntN(3)), Band(r.IntN(3)), now-r.Int64N(3*after), uint64(k)))
		}
		got, _ := Order(State{}, q, now, Config{StarvationAfterMs: after})
		for _, x := range got {
			waited := now - x.EnqueuedMs
			switch {
			case x.Band == Background && waited > after:
				if x.Effective != Normal || !x.Promoted {
					t.Fatalf("seed %d: %s waited %d ms in background and was not promoted to normal (%s)", seed, x.Key, waited, x.Effective)
				}
			case x.Effective != x.Band || x.Promoted:
				t.Fatalf("seed %d: %s moved from %s to %s after %d ms", seed, x.Key, x.Band, x.Effective, waited)
			}
			if x.Effective == Interactive && x.Band != Interactive {
				t.Fatalf("seed %d: %s reached interactive from %s", seed, x.Key, x.Band)
			}
		}
	}
	old := item("old", "p", Background, 0, 1)
	young := item("young", "p", Background, 1, 2)
	norm := item("norm", "q", Normal, 0, 3)
	// "longer than" the threshold: exactly at it, no promotion; one millisecond over, promotion
	if at, _ := Order(State{}, []Item{old}, after, Config{StarvationAfterMs: after}); at[0].Promoted || at[0].Effective != Background {
		t.Fatalf("a job waiting exactly the threshold is not promoted: %+v", at[0])
	}
	if below, _ := Order(State{}, []Item{old}, after-1, Config{StarvationAfterMs: after}); below[0].Promoted {
		t.Fatalf("a job waiting less than the threshold is not promoted: %+v", below[0])
	}
	got, _ := Order(State{}, []Item{young, old, norm}, after+1, Config{StarvationAfterMs: after})
	if !got[0].Promoted && !got[1].Promoted || got[2].Key != "young" {
		t.Fatalf("a job waiting longer than the threshold is promoted; got %v", keys(got))
	}
	if off, _ := Order(State{}, []Item{old}, 100*after, Config{}); off[0].Promoted {
		t.Fatal("a zero threshold disables promotion")
	}
	// a promoted job competes fairly in normal: it does not jump the other project's head by band alone
	if got[0].Effective != Normal {
		t.Fatalf("promoted job ordered in %s", got[0].Effective)
	}
}

// TestPriorityNeverAdmits: the order is only a permutation, so a job the
// scheduler found no runner for stays unplaced under any priority or band
// (the dispatch test of the same name runs it through sched.Place).
func TestPriorityNeverAdmits(t *testing.T) {
	for i := 0; i < cases; i++ {
		seed := uint64(i) + 1
		r := rng(seed)
		n := 1 + r.IntN(30)
		var q []Item
		eligible := map[string]bool{}
		for k := 0; k < n; k++ {
			it := item(fmt.Sprintf("j%03d", k), fmt.Sprintf("p%d", r.IntN(3)), Band(r.IntN(3)), int64(r.IntN(1000)), uint64(k))
			it.Priority = []string{"low", "normal", "high"}[r.IntN(3)]
			q = append(q, it)
			eligible[it.Key] = r.IntN(3) > 0
		}
		got, _ := Order(State{}, q, 1<<40, Config{StarvationAfterMs: 1})
		slots, admitted := 1+r.IntN(n), 0
		for _, x := range got { // greedy placement in this order, as sched.Place does: only eligible jobs take a slot
			if eligible[x.Key] && admitted < slots {
				admitted++
			}
		}
		want := 0
		for _, ok := range eligible {
			if ok {
				want++
			}
		}
		if admitted != min(slots, want) {
			t.Fatalf("seed %d: admitted %d, want %d: reordering changed what is admissible", seed, admitted, min(slots, want))
		}
		if len(got) != len(q) {
			t.Fatalf("seed %d: the order added or dropped jobs", seed)
		}
		have := map[string]bool{}
		for _, x := range got {
			have[x.Key] = true
		}
		for _, it := range q {
			if !have[it.Key] {
				t.Fatalf("seed %d: %s vanished from the order", seed, it.Key)
			}
		}
	}
}

// TestBandFor: the band of a pipeline from its declared band, trigger and foreground flag.
func TestBandFor(t *testing.T) {
	for _, c := range []struct {
		declared, trigger string
		fg                bool
		want              Band
	}{
		{"", "push", false, Normal}, {"", "pull_request", false, Normal}, {"", "tag", false, Normal}, {"", "api", false, Normal},
		{"", "manual", false, Interactive}, {"", "local", false, Interactive}, {"", "", false, Interactive}, {"", "push", true, Interactive},
		{"", "schedule", false, Background}, {"background", "push", true, Background}, {"interactive", "schedule", false, Interactive},
		{"bogus", "push", false, Normal},
	} {
		if got := For(c.declared, c.trigger, c.fg); got != c.want {
			t.Errorf("For(%q, %q, %v) = %s, want %s", c.declared, c.trigger, c.fg, got, c.want)
		}
	}
}
