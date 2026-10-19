package fairq

// Purpose: the order in which queued jobs reach the scheduler (Epic D9). Strict
// band order; inside a band, start-time fair queuing across projects with equal
// weights; inside a project, job priority (high > normal > low) then age, then
// the stable tie-break (pipeline id, job id, matrix key). A job that waited
// longer than (strictly) the starvation threshold moves up one band and never above
// normal (R-AR-27).
// Inputs:  the previous State, the queued Items, the clock as a value (ms) and a Config.
// Outputs: the Ranked order and the next State.
// Constraints: pure and deterministic (no clock, no I/O, no goroutines, no map
// iteration order, integer arithmetic only). It only reorders: it never drops,
// adds or marks a job eligible, so priority cannot admit what the scheduler
// refused. Virtual time: a project's virtual start is max(its virtual finish,
// the band's virtual time). The state advances when a job that was queued at the
// previous call is gone at this one (it was admitted, or its caller gave up);
// call Order once per tick with the whole live queue.

import (
	"cmp"
	"math"
	"slices"
	"sort"
)

// Item is one queued job.
type Item struct {
	Key                               string // unique among the queued items
	Pipeline, Job, MatrixKey, Project string
	// Band is the band the job was submitted in; Priority is high, normal or low.
	Band     Band
	Priority string
	// EnqueuedMs is when the job entered the queue (unix ms); Seq is arrival order.
	EnqueuedMs int64
	Seq        uint64
	// Cost is the weight of the job in virtual time (an estimated runtime in ms,
	// or 1 for plain job counting). Zero or negative counts as 1.
	Cost int64
}

// Config is the fairness policy.
type Config struct {
	// StarvationAfterMs is how long a job waits before it moves up one band; 0 disables promotion.
	StarvationAfterMs int64
}

// Ranked is one job in the order.
type Ranked struct {
	Item
	// Effective is the band the job was ordered in; Promoted says starvation moved it.
	Effective Band
	Promoted  bool
	// Position is the 0-based place in the order.
	Position int
}

type bp struct {
	band    Band
	project string
}

type seenItem struct {
	bp   bp
	cost int64
	rank int
}

// State is the virtual clock of the fair queue. The zero value is an empty
// queue history. A State is a value: Order returns the next one and never
// changes its argument.
type State struct {
	vt   [bandCount]int64
	fin  map[bp]int64
	seen map[string]seenItem
}

// entry is an item on its way through Order: the band it was submitted in and
// whether starvation moved it.
type entry struct {
	Item
	declared Band
	promoted bool
}

// Order ranks items; see the file comment. prev is not modified.
func Order(prev State, items []Item, nowMs int64, cfg Config) ([]Ranked, State) {
	st := prev.depart(items)
	var byBand [bandCount][]entry
	for _, it := range items {
		e := entry{Item: it, declared: it.Band}
		if e.declared < Interactive || e.declared >= bandCount {
			e.declared = Normal
		}
		b := e.declared
		if b == Background && cfg.StarvationAfterMs > 0 && nowMs-it.EnqueuedMs > cfg.StarvationAfterMs {
			b, e.promoted = Normal, true
		}
		byBand[b] = append(byBand[b], e)
	}
	out := make([]Ranked, 0, len(items))
	for b := Interactive; b < bandCount; b++ {
		for _, e := range st.serve(b, byBand[b]) {
			it := e.Item
			it.Band = e.declared // Band is the band the job was submitted in; Effective is where it was ordered
			out = append(out, Ranked{Item: it, Effective: b, Promoted: e.promoted, Position: len(out)})
		}
	}
	st.seen = make(map[string]seenItem, len(out))
	for _, r := range out {
		st.seen[r.Key] = seenItem{bp{r.Effective, r.Project}, cost(r.Cost), r.Position}
	}
	return out, st
}

// satAdd adds two non-negative values and stops at the largest int64, so an
// absurd cost can never wrap virtual time round to a small number.
func satAdd(a, b int64) int64 {
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

func cost(c int64) int64 {
	if c < 1 {
		return 1
	}
	return c
}

// depart charges the projects of the jobs that left the queue since the last
// call, in the order they were ranked, and returns the advanced state.
func (s State) depart(now []Item) State {
	st := State{vt: s.vt, fin: make(map[bp]int64, len(s.fin))}
	for k, v := range s.fin {
		st.fin[k] = v
	}
	live := make(map[string]bool, len(now))
	for _, it := range now {
		live[it.Key] = true
	}
	var gone []seenItem
	for k, v := range s.seen {
		if !live[k] {
			gone = append(gone, v)
		}
	}
	sort.Slice(gone, func(i, j int) bool { return gone[i].rank < gone[j].rank })
	for _, g := range gone {
		start := max(st.fin[g.bp], st.vt[g.bp.band])
		st.fin[g.bp] = satAdd(start, g.cost)
		st.vt[g.bp.band] = max(st.vt[g.bp.band], start)
	}
	// A project whose finish is behind the band's virtual time carries no
	// information: its next virtual start is the band's anyway.
	for k, v := range st.fin {
		if v <= st.vt[k.band] {
			delete(st.fin, k)
		}
	}
	return st
}

// serve interleaves the jobs of one band: repeatedly the project with the
// smallest virtual start emits its next job (priority, age, tie-break), and its
// virtual finish advances by that job's cost. It works on copies: the real
// state moves only when jobs depart.
func (s State) serve(b Band, items []entry) []entry {
	if len(items) == 0 {
		return nil
	}
	queues := map[string][]entry{}
	var projects []string
	for _, it := range items {
		if _, ok := queues[it.Project]; !ok {
			projects = append(projects, it.Project)
		}
		queues[it.Project] = append(queues[it.Project], it)
	}
	sort.Strings(projects)
	fin := make(map[string]int64, len(projects))
	for _, p := range projects {
		q := queues[p]
		slices.SortFunc(q, compare)
		fin[p] = s.fin[bp{b, p}]
	}
	v := s.vt[b]
	out := make([]entry, 0, len(items))
	for len(out) < len(items) {
		best, bestStart, found := "", int64(0), false
		for _, p := range projects {
			if len(queues[p]) == 0 {
				continue
			}
			if st := max(fin[p], v); !found || st < bestStart {
				best, bestStart, found = p, st, true
			}
		}
		head := queues[best][0]
		queues[best] = queues[best][1:]
		out = append(out, head)
		fin[best], v = satAdd(bestStart, cost(head.Cost)), bestStart
	}
	return out
}

// compare is the order inside one project: priority, age, then the stable tie-break.
func compare(a, b entry) int {
	return cmp.Or(
		cmp.Compare(rank(a.Priority), rank(b.Priority)),
		cmp.Compare(a.EnqueuedMs, b.EnqueuedMs),
		cmp.Compare(a.Pipeline, b.Pipeline),
		cmp.Compare(a.Job, b.Job),
		cmp.Compare(a.MatrixKey, b.MatrixKey),
		cmp.Compare(a.Seq, b.Seq),
		cmp.Compare(a.Key, b.Key),
	)
}
