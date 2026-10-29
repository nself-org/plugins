package stats

// Purpose: the per-attempt blocks of contract:ci.stats: runner utilisation, outcome rates, job durations and
// ephemeral provisioning latency.
// Inputs:  the finished-attempt telemetry rows of the window.
// Outputs: Doc.Runners, Attempts, Jobs, Provisioning.
// Constraints: only passed and failed attempts count; a failure of class infra is an infra retry, any other
//          failure marks its job as failed-before, and a later pass of that job makes the job flaky.

import (
	"math"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

func (d *Doc) buildRuns(rows store.StatsRows, out []store.StatOutcome, o Options) {
	if rows.Missing["schedtel_outcome"] {
		for _, k := range []string{"runners", "attempts", "jobs", "provisioning_ms"} {
			d.Reasons[k] = ReasonNotInstalled
		}
		return
	}
	if len(out) == 0 {
		for _, k := range []string{"runners", "attempts", "jobs", "provisioning_ms"} {
			d.Reasons[k] = ReasonNoSamples
		}
		return
	}
	slots := nodeSlots(rows.Nodes)
	window := o.Since.Milliseconds()
	d.Runners, d.Jobs = map[string]Runner{}, map[string]Pct{}
	runtimes := map[string][]int64{}
	var cold []int64
	var passed, failed, infra int64
	// A job is flaky when a non-infra failure precedes a pass: its first such failure comes before its last pass.
	type span struct{ firstFail, lastPass int }
	jobs := make(map[string]span, len(out))
	for _, x := range out {
		s, ok := jobs[x.Job]
		if !ok {
			s = span{firstFail: math.MaxInt, lastPass: math.MinInt}
		}
		r := d.Runners[x.Runner]
		r.Attempts++
		if x.RuntimeMs >= 0 {
			r.BusyMs += x.RuntimeMs
			runtimes[x.JobKey] = append(runtimes[x.JobKey], x.RuntimeMs)
		}
		d.Runners[x.Runner] = r
		if x.RunnerClass == "ephemeral" && x.ColdStartMs >= 0 {
			cold = append(cold, x.ColdStartMs)
		}
		switch {
		case x.Result == "passed":
			passed++
			s.lastPass = max(s.lastPass, x.N)
		case x.FailureClass == "infra":
			failed++
			infra++
		default:
			failed++
			s.firstFail = min(s.firstFail, x.N)
		}
		jobs[x.Job] = s
	}
	var flaky int64
	for _, s := range jobs {
		if s.firstFail < s.lastPass {
			flaky++
		}
	}
	for id, r := range d.Runners {
		s, ok := slots[id]
		if !ok || s <= 0 {
			d.Reasons["runners."+id+".utilisation_permille"] = ReasonSlotsUnknown
			continue
		}
		avail := int64(s) * window
		u := min(1000, permille(r.BusyMs, avail))
		r.Slots, r.AvailableMs, r.UtilisationPermille = &s, &avail, &u
		d.Runners[id] = r
	}
	total := passed + failed
	d.Attempts = &Attempts{Finished: int(total), PassPermille: permille(passed, total), FailPermille: permille(failed, total),
		FlakyPermille: permille(flaky, int64(len(jobs))), InfraRetryPermille: permille(infra, total)}
	for k, v := range runtimes {
		d.Jobs[k] = *pct(v)
	}
	if len(d.Jobs) == 0 {
		d.Jobs = nil
		d.Reasons["jobs"] = ReasonNoSamples
	}
	if d.Provisioning = pct(cold); d.Provisioning == nil {
		d.Reasons["provisioning_ms"] = ReasonNoSamples
	}
}
