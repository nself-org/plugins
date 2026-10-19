package detach

// Purpose: a ci.events stream for a run that has no event file (P7-CI-63): a
// run started in the foreground, or one that lives on a coordinator. The
// records are rebuilt from the run's observable state (store rows or a
// RemoteResolver view), so `runs follow` has one output shape for every run.
// Inputs:  successive View snapshots of the run.
// Outputs: queued, started, check_result, finished and the final evidence line,
// written by internal/events (seq strictly increasing from 1).
// Constraints: a run always rebuilds to the same records and seqs (live, only
// `queued` is written; the rest comes once the run is over); records carry only
// what a View holds (no log bytes, no
// timings); the first record is always `queued`; lines below the requested seq
// are dropped, the final line never is.

import (
	"bytes"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/events"
)

// JobState is one job of a View.
type JobState struct {
	Name, State, FailureClass string
}

// View is a snapshot of a run.
type View struct {
	RunID string
	State string // pipeline status: running, passed, failed, cancelled, lost, …
	Jobs  []JobState
}

// Terminal reports whether a pipeline status is final.
func Terminal(state string) bool {
	switch state {
	case "", "queued", "running":
		return false
	}
	return true
}

// Synth turns View snapshots into a record stream. While the run is live it
// writes only `queued`; everything else is written once the run is over, in one
// fixed order (jobs sorted by name: started, check_result; then finished and
// the result), so the seqs of a run never depend on when it was observed and a
// client that resumes with --from-seq always lands on the same records.
type Synth struct {
	em   *events.Emitter
	open bool
}

// NewSynth writes the records of run runID to w, from seq `from` on.
func NewSynth(w io.Writer, runID string, from int64) *Synth {
	return &Synth{em: events.New(&fromFilter{w: w, from: from}, "ci", runID, runID, nil)}
}

// Observe announces the run (`queued`, seq 1) the first time it is called.
func (s *Synth) Observe(View) {
	if !s.open {
		s.open = true
		s.em.Emit("queued", "", "", map[string]any{})
	}
}

// Finish emits the records of the finished run, `finished`, and the closing
// evidence line.
func (s *Synth) Finish(v View, evidence any) {
	s.Observe(v)
	jobs := append([]JobState(nil), v.Jobs...)
	sort.Slice(jobs, func(i, k int) bool { return jobs[i].Name < jobs[k].Name })
	for _, j := range jobs {
		switch j.State {
		case "passed", "failed", "cancelled":
			s.em.Emit("started", j.Name, "", map[string]any{})
			fallthrough
		case "skipped":
			s.em.Emit("check_result", j.Name, "", map[string]any{"state": j.State, "check": j.Name, "failure_class": j.FailureClass})
		}
	}
	s.em.Emit("finished", "", "", map[string]any{"result": v.State, "jobs": len(v.Jobs)})
	s.em.End(evidence, nil)
}

// Flush waits until everything emitted so far was written.
func (s *Synth) Flush() { s.em.Flush() }

// ResultLine renders the closing line of a stream, holding the evidence, with
// the given seq (the same layout internal/events writes).
func ResultLine(runID string, seq int64, evidence any) ([]byte, error) {
	return output.StreamEnd("ci", nil,
		output.Field{Key: "schema", Value: events.Schema}, output.Field{Key: "seq", Value: seq},
		output.Field{Key: "ts", Value: time.Now().UTC().Format(time.RFC3339Nano)},
		output.Field{Key: "run_id", Value: runID}, output.Field{Key: "pipeline_id", Value: runID},
		output.Field{Key: "event", Value: "evidence"}, output.Field{Key: "evidence", Value: evidence})
}

// fromFilter drops complete lines whose seq is below from (the emitter writes
// one whole line per Write call).
type fromFilter struct {
	mu   sync.Mutex
	w    io.Writer
	from int64
}

func (f *fromFilter) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if rec, ok := ParseLine(bytes.TrimRight(p, "\n")); ok && !rec.Final() && rec.Seq < f.from {
		return len(p), nil
	}
	return f.w.Write(p)
}
