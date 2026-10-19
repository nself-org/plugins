package detach

// Purpose: a ci.events stream for a run that has no event file (P7-CI-63): a
// run started in the foreground, or one that lives on a coordinator. The
// records are rebuilt from the run's observable state (store rows or a
// RemoteResolver view), so `runs follow` has one output shape for every run.
// Inputs:  successive View snapshots of the run.
// Outputs: queued, started, check_result, finished and the final evidence line,
// written by internal/events (seq strictly increasing from 1).
// Constraints: records carry only what a View holds (no log bytes, no
// timings); the first record is always `queued`; lines below the requested seq
// are dropped, the final line never is.

import (
	"bytes"
	"io"
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

// Synth turns View snapshots into a record stream.
type Synth struct {
	em   *events.Emitter
	sent map[string]string
	open bool
}

// NewSynth writes the records of run runID to w, from seq `from` on.
func NewSynth(w io.Writer, runID string, from int64) *Synth {
	return &Synth{em: events.New(&fromFilter{w: w, from: from}, "ci", runID, runID, nil), sent: map[string]string{}}
}

// Observe emits what changed since the last snapshot.
func (s *Synth) Observe(v View) {
	if !s.open {
		s.open = true
		s.em.Emit("queued", "", "", map[string]any{})
	}
	for _, j := range v.Jobs {
		if s.sent[j.Name] == j.State {
			continue
		}
		switch j.State {
		case "leased", "running", "finalizing":
			if s.sent[j.Name] == "" || s.sent[j.Name] == "queued" {
				s.em.Emit("started", j.Name, "", map[string]any{})
			}
		case "passed", "failed", "skipped", "cancelled":
			s.em.Emit("check_result", j.Name, "", map[string]any{"state": j.State, "check": j.Name, "failure_class": j.FailureClass})
		default:
			continue
		}
		s.sent[j.Name] = j.State
	}
}

// Finish emits the final snapshot, `finished`, and the closing evidence line.
func (s *Synth) Finish(v View, evidence any) {
	s.Observe(v)
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
