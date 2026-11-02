package stats

// Purpose: the per-attempt blocks of contract:ci.stats: runner utilisation, outcome rates, job durations and
// ephemeral provisioning latency.
// Inputs:  the finished-attempt telemetry rows of the window.
// Outputs: Doc.Runners, Attempts, Jobs, Provisioning.
// Constraints: only passed and failed attempts count; a failure of class infra is an infra retry, any other
//          failure marks its job as failed-before, and a later pass of that job makes the job flaky.

import "github.com/nself-org/plugins/free/ci/internal/store"

func (d *Doc) buildRuns(rows store.StatsRows, o Options) {
	if rows.Missing["schedtel_outcome"] {
		for _, k := range []string{"runners", "attempts", "jobs", "provisioning_ms"} {
			d.Reasons[k] = ReasonNotInstalled
		}
		return
	}
	var passed, failed, infra int64
	runners := map[string]Runner{}
	for _, x := range rows.Runs {
		if !o.visible(x.Project) {
			continue
		}
		r := runners[x.Runner]
		r.Attempts += int(x.N)
		r.BusyMs += x.BusyMs
		runners[x.Runner] = r
		switch {
		case x.Result == "passed":
			passed += x.N
		case x.FailureClass == "infra":
			failed += x.N
			infra += x.N
		default:
			failed += x.N
		}
	}
	total := passed + failed
	if total == 0 {
		for _, k := range []string{"runners", "attempts", "jobs", "provisioning_ms"} {
			d.Reasons[k] = ReasonNoSamples
		}
		return
	}
	slots := nodeSlots(rows.Nodes)
	window := o.Since.Milliseconds()
	d.Runners, d.Jobs = runners, map[string]Pct{}
	// A job is flaky when a non-infra failure precedes a pass: the store counts, per project, the jobs whose
	// first such failure comes before their last pass.
	var jobs, flaky int64
	for _, j := range rows.Jobs {
		if o.visible(j.Project) {
			jobs += j.Jobs
			flaky += j.Flaky
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
	d.Attempts = &Attempts{Finished: int(total), PassPermille: permille(passed, total), FailPermille: permille(failed, total),
		FlakyPermille: permille(flaky, max(jobs, 1)), InfraRetryPermille: permille(infra, total)}
	runtimes := map[string][]store.StatSample{}
	for _, x := range rows.Runtimes {
		if o.visible(x.Project) {
			runtimes[x.Key] = append(runtimes[x.Key], x)
		}
	}
	for k, v := range runtimes {
		d.Jobs[k] = *pctSamples(v)
	}
	if len(d.Jobs) == 0 {
		d.Jobs = nil
		d.Reasons["jobs"] = ReasonNoSamples
	}
	var cold []store.StatSample
	for _, x := range rows.Colds {
		if o.visible(x.Project) {
			cold = append(cold, x)
		}
	}
	if d.Provisioning = pctSamples(cold); d.Provisioning == nil {
		d.Reasons["provisioning_ms"] = ReasonNoSamples
	}
}
