package stats

// Purpose: assembles contract:ci.stats v1 from the rows internal/store reads (P7-SCHED-20, Epic SCHED D23).
// Inputs:  Options (clock, window, project, visibility) and a store, or the rows themselves.
// Outputs: Doc.
// Constraints: read-only; a source that is absent or has no rows is null with a reason, never 0; the queue
//          wait of a band is the telemetry wait of the finished attempts of that band; a project-scoped
//          caller sees only coordinator-wide blocks that carry a project (leases, cache, spend, storage).

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/fairq"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Options selects what Compute reads.
type Options struct {
	Now     time.Time
	Since   time.Duration // default 24h
	Project string        // "" = all visible projects
	// Allow reports whether the caller may read a project; nil allows all. Global says the caller may
	// read the whole coordinator (all projects), which the blocks that carry no project need.
	Allow  func(string) bool
	Global bool
}

// DefaultSince is the window when none is given.
const DefaultSince = 24 * time.Hour

func (o Options) visible(project string) bool {
	if o.Project != "" && project != o.Project {
		return false
	}
	if project == "" {
		return o.Global
	}
	return o.Allow == nil || o.Allow(project)
}

// Compute reads st in one read transaction and builds the document.
func Compute(ctx context.Context, st *store.Store, o Options) (Doc, error) {
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	if o.Since <= 0 {
		o.Since = DefaultSince
	}
	rows, err := st.StatsRead(ctx, store.StatsQuery{Now: o.Now, Since: o.Since, Project: o.Project})
	if err != nil {
		return Doc{}, err
	}
	return Build(rows, o), nil
}

// Build assembles the document from rows.
func Build(rows store.StatsRows, o Options) Doc {
	if o.Since <= 0 {
		o.Since = DefaultSince
	}
	d := Doc{Contract: Contract, Version: Version, GeneratedAt: o.Now.UTC().Format(time.RFC3339), SinceMs: o.Since.Milliseconds(), Reasons: map[string]string{}}
	if o.Project != "" {
		p := o.Project
		d.Project = &p
	}
	out := make([]store.StatOutcome, 0, len(rows.Outcomes))
	for _, x := range rows.Outcomes {
		if o.visible(x.Project) && (x.Result == "passed" || x.Result == "failed") {
			out = append(out, x)
		}
	}
	d.buildQueue(rows, out, o)
	d.buildRuns(rows, out, o)
	d.buildCacheSpend(rows, o)
	d.buildPlatform(rows, o)
	return d
}

func band(trigger string) string { return fairq.For("", trigger, false).String() }

func (d *Doc) buildQueue(rows store.StatsRows, out []store.StatOutcome, o Options) {
	depth := map[string]int{}
	waits := map[string][]int64{}
	for _, g := range rows.Queued {
		if o.visible(g.Project) {
			depth[band(g.Key)] += int(g.N)
		}
	}
	bands := map[string]string{} // a trigger's band, worked out once per trigger instead of once per attempt
	for _, x := range out {
		if x.WaitMs >= 0 {
			b, ok := bands[x.Trigger]
			if !ok {
				b = band(x.Trigger)
				bands[x.Trigger] = b
			}
			waits[b] = append(waits[b], x.WaitMs)
		}
	}
	if rows.Missing["schedtel_outcome"] && len(depth) == 0 {
		d.Reasons["queue"] = ReasonNotInstalled
		return
	}
	if len(depth) == 0 && len(waits) == 0 {
		d.Reasons["queue"] = ReasonNoSamples
		return
	}
	d.Queue = map[string]Band{}
	for _, b := range []string{"interactive", "normal", "background"} {
		if depth[b] == 0 && len(waits[b]) == 0 {
			continue
		}
		d.Queue[b] = Band{Depth: depth[b], WaitMs: pct(waits[b])}
		if len(waits[b]) == 0 {
			d.Reasons["queue."+b+".wait_ms"] = ReasonNoSamples
		}
	}
}

// NoStoreRows stands for a machine with no run store: every optional set reads as not installed.
func NoStoreRows() store.StatsRows {
	m := map[string]bool{}
	for _, t := range []string{"schedtel_outcome", "ledger", "cache_use", "cache_entry", "cache_artifact", "outbox", "node"} {
		m[t] = true
	}
	return store.StatsRows{Missing: m}
}
