package store

// Purpose: folds the finished-attempt telemetry rows of the stats window into groups while they are scanned
// (P7-SCHED-20), so a window of 100 000 attempts is never held as 100 000 rows of a dozen strings each.
// Inputs:  the rows of the "outcomes" statement of StatsStatements.
// Outputs: StatsRows.Runs, Jobs, Waits, Runtimes and Colds.
// Constraints: only passed and failed attempts arrive (the statement filters); a sample counts only when it is
//          non-negative (a negative figure is "not measured"); a job is flaky when its first non-infra failure
//          comes before its last pass; the groups are exact, so the percentiles built from them equal the ones
//          built from the rows.

import (
	"database/sql"
	"math"
)

// The bits of the last column of the "outcomes" statement.
const (
	flagPassed    = 1 // result = passed
	flagInfra     = 2 // failure class = infra
	flagEphemeral = 4 // runner class = ephemeral
)

type runKey struct{ project, runner, result, failure string }

type sampleKey struct {
	project, key string
	ms           int64
}

type jobSpan struct {
	project             string
	firstFail, lastPass int64
}

type outcomeFold struct {
	runs                   map[runKey]*StatRun
	jobs                   map[string]jobSpan
	waits, runtimes, colds map[sampleKey]int64
}

func foldOutcomes(rows *sql.Rows, out *StatsRows) error {
	f := &outcomeFold{runs: map[runKey]*StatRun{}, jobs: map[string]jobSpan{},
		waits: map[sampleKey]int64{}, runtimes: map[sampleKey]int64{}, colds: map[sampleKey]int64{}}
	var job, project, jobKey, runner, trigger string
	var n, wait, runtime, cold, flags int64
	for rows.Next() {
		if err := rows.Scan(&job, &n, &project, &jobKey, &runner, &trigger, &wait, &runtime, &cold, &flags); err != nil {
			return err
		}
		// The three enumerations arrive as one small number: each text column costs a string per row.
		result, failure := "failed", ""
		if flags&flagPassed != 0 {
			result = "passed"
		}
		if flags&flagInfra != 0 {
			failure = "infra"
		}
		k := runKey{project, runner, result, failure}
		r := f.runs[k]
		if r == nil {
			r = &StatRun{Project: project, Runner: runner, Result: result, FailureClass: failure}
			f.runs[k] = r
		}
		r.N++
		if runtime >= 0 {
			r.BusyMs += runtime
			f.runtimes[sampleKey{project, jobKey, runtime}]++
		}
		if wait >= 0 {
			f.waits[sampleKey{project, trigger, wait}]++
		}
		if flags&flagEphemeral != 0 && cold >= 0 {
			f.colds[sampleKey{project, "", cold}]++
		}
		s, ok := f.jobs[job]
		if !ok {
			s = jobSpan{project, math.MaxInt64, math.MinInt64}
		}
		switch {
		case flags&flagPassed != 0:
			s.lastPass = max(s.lastPass, n)
		case flags&flagInfra == 0:
			s.firstFail = min(s.firstFail, n)
		}
		f.jobs[job] = s
	}
	f.collect(out)
	return nil
}

// collect moves the folded groups into out.
func (f *outcomeFold) collect(out *StatsRows) {
	for _, r := range f.runs {
		out.Runs = append(out.Runs, *r)
	}
	perProject := map[string]*StatJobs{}
	for _, s := range f.jobs {
		j := perProject[s.project]
		if j == nil {
			j = &StatJobs{Project: s.project}
			perProject[s.project] = j
		}
		j.Jobs++
		if s.firstFail < s.lastPass {
			j.Flaky++
		}
	}
	for _, j := range perProject {
		out.Jobs = append(out.Jobs, *j)
	}
	samples := func(m map[sampleKey]int64) []StatSample {
		v := make([]StatSample, 0, len(m))
		for k, n := range m {
			v = append(v, StatSample{Project: k.project, Key: k.key, Ms: k.ms, N: n})
		}
		return v
	}
	out.Waits, out.Runtimes, out.Colds = samples(f.waits), samples(f.runtimes), samples(f.colds)
}
