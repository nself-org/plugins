package store

// Purpose: folds the finished-attempt telemetry rows of the stats window into groups while they are scanned
// (P7-SCHED-20), so a window of 100 000 attempts is never held as 100 000 rows of a dozen strings each.
// Inputs:  the rows of the "outcomes" statement of StatsStatements: one packed text cell each, because the
//          driver costs about as much per cell as SQLite costs per column; "outcomes_cells" is the same
//          statement as separate columns, read only when a text field holds the separator.
// Outputs: StatsRows.Runs, Jobs, Waits, Runtimes and Colds.
// Constraints: only passed and failed attempts arrive (the statement filters); a sample counts only when it is
//          non-negative (a negative figure is "not measured"); a job is flaky when its first non-infra failure
//          comes before its last pass; the groups are exact, so the percentiles built from them equal the ones
//          built from the rows.

import (
	"database/sql"
	"errors"
	"math"
	"strings"
)

// The bits of the last column of the "outcomes" statement.
const (
	flagPassed    = 1 // result = passed
	flagInfra     = 2 // failure class = infra
	flagEphemeral = 4 // runner class = ephemeral
)

// sep splits the fields of a packed row; errPacked says a row did not split into them.
const sep = 0x1f

var errPacked = errors.New("stats: an outcome row does not split into its fields")

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

func newOutcomeFold() *outcomeFold {
	return &outcomeFold{runs: map[runKey]*StatRun{}, jobs: map[string]jobSpan{},
		waits: map[sampleKey]int64{}, runtimes: map[sampleKey]int64{}, colds: map[sampleKey]int64{}}
}

// foldOutcomes folds the "outcomes" statement: one packed text cell per attempt (see packedOutcomes).
// It returns errPacked when a row does not split into its fields, and then out is untouched.
func foldOutcomes(rows *sql.Rows, out *StatsRows) error {
	f := newOutcomeFold()
	var line string
	for rows.Next() {
		if err := rows.Scan(&line); err != nil {
			return err
		}
		if err := f.addPacked(line); err != nil {
			return err
		}
	}
	f.collect(out)
	return nil
}

// foldOutcomeCells folds the "outcomes_cells" statement: the same fields as separate columns.
func foldOutcomeCells(rows *sql.Rows, out *StatsRows) error {
	f := newOutcomeFold()
	var job, project, jobKey, runner, trigger string
	var n, wait, runtime, cold, flags int64
	for rows.Next() {
		if err := rows.Scan(&n, &wait, &runtime, &cold, &flags, &project, &runner, &trigger, &jobKey, &job); err != nil {
			return err
		}
		f.add(job, n, project, jobKey, runner, trigger, wait, runtime, cold, flags)
	}
	f.collect(out)
	return nil
}

// addPacked splits one packed row: five integers, then project, runner, trigger and job key, then the job id
// (last, so it is the only field that could hold a separator; any row with more separators is refused).
func (f *outcomeFold) addPacked(s string) error {
	var v [5]int64
	for i := range v {
		j := strings.IndexByte(s, sep)
		if j < 0 {
			return errPacked
		}
		n, ok := atoi(s[:j])
		if !ok {
			return errPacked
		}
		v[i], s = n, s[j+1:]
	}
	var t [4]string
	for i := range t {
		j := strings.IndexByte(s, sep)
		if j < 0 {
			return errPacked
		}
		t[i], s = s[:j], s[j+1:]
	}
	if strings.IndexByte(s, sep) >= 0 {
		return errPacked
	}
	f.add(s, v[0], t[0], t[3], t[1], t[2], v[1], v[2], v[3], v[4])
	return nil
}

// atoi parses a decimal integer with an optional minus sign, without allocating.
func atoi(s string) (int64, bool) {
	neg := len(s) > 0 && s[0] == '-'
	if neg {
		s = s[1:]
	}
	if s == "" || len(s) > 18 {
		return 0, false
	}
	var n int64
	for i := 0; i < len(s); i++ {
		d := s[i] - '0'
		if d > 9 {
			return 0, false
		}
		n = n*10 + int64(d)
	}
	if neg {
		n = -n
	}
	return n, true
}

// add folds one attempt. The keys of the sample and run groups may alias the row they were cut from (a group
// outlives its first row by design and there are few of them); a job id is copied, there is one per job.
func (f *outcomeFold) add(job string, n int64, project, jobKey, runner, trigger string, wait, runtime, cold, flags int64) {
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
		job, s = strings.Clone(job), jobSpan{project, math.MaxInt64, math.MinInt64}
	}
	switch {
	case flags&flagPassed != 0:
		s.lastPass = max(s.lastPass, n)
	case flags&flagInfra == 0:
		s.firstFail = min(s.firstFail, n)
	}
	f.jobs[job] = s
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
