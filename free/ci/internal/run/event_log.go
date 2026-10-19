package run

// Purpose: the contract:ci.events records of a run, and their store (D-1061).
// Records maps an engine Event to the records a stream carries; logged wraps
// the run's renderer so every record is also appended to the store's event
// log, whatever renderer the run uses. `ci runs follow` replays that log for a
// run that has no event file.
// Inputs:  Events and the final Summary; the run's store.
// Outputs: events.Record values; rows in event_index.
// Constraints: a log failure never fails or slows the verdict (the append is
// best effort, in event order); `log` and `retrying` engine events have no
// record; the mapping is the one the --json --stream renderer prints.

import (
	"context"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/events"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Records returns the stream records of an engine event: none for events that
// are not part of the stream (log, retrying).
func (e Event) Records() []events.Record {
	switch e.Type {
	case "queued":
		return []events.Record{{Type: "queued", Data: map[string]any{}}}
	case "planned":
		return []events.Record{{Type: "planned", Data: plannedData(e)}}
	case "started":
		return []events.Record{{Type: "started", Job: e.Job, Data: map[string]any{}}}
	case "finished":
		o := e.Out
		if o == nil {
			return nil
		}
		out := []events.Record{{Type: "check_result", Job: e.Job, Attempt: o.AttemptID, Data: map[string]any{"state": o.State, "check": o.Check, "required": o.Required,
			"reason": string(o.Reason), "failure_class": string(o.FailureClass), "exit_code": o.ExitCode, "duration_ms": o.Duration.Milliseconds()}}}
		if o.State == StateCancelled {
			out = append(out, events.Record{Type: "cancelled", Job: e.Job, Attempt: o.AttemptID, Data: map[string]any{}})
		}
		return out
	}
	return nil
}

// Record is the closing `finished` record of a run.
func (s Summary) Record() events.Record {
	return events.Record{Type: "finished", Data: map[string]any{"result": string(s.Result), "jobs": len(s.Jobs)}}
}

func plannedData(e Event) map[string]any {
	jobs := []map[string]any{}
	skipped := []map[string]any{}
	if e.Plan != nil {
		for _, j := range e.Plan.Jobs {
			jobs = append(jobs, map[string]any{"id": j.ID, "check": j.Check, "state": j.State, "reason": string(j.Reason), "blocked": string(j.Blocked)})
		}
		for _, k := range e.Plan.Skipped {
			skipped = append(skipped, map[string]any{"job": k.Job, "reason": string(k.Reason)})
		}
	}
	return map[string]any{"jobs": jobs, "skipped": skipped}
}

// logged appends the records of every event to the store before the wrapped
// renderer sees the event.
type logged struct {
	Renderer
	st    *store.Store
	runID string
	mu    sync.Mutex
}

// logEvents wraps r so the run's records are stored; a nil store changes nothing.
func logEvents(r Renderer, st *store.Store, runID string) Renderer {
	if st == nil {
		return r
	}
	return &logged{Renderer: r, st: st, runID: runID}
}

func (l *logged) append(recs ...events.Record) {
	l.mu.Lock()
	defer l.mu.Unlock()
	ctx, done := background(context.Background())
	defer done()
	for _, r := range recs {
		_, _ = l.st.AppendEvent(ctx, l.runID, string(r.Type), r.Ref())
	}
}

func (l *logged) Event(e Event) {
	l.append(e.Records()...)
	l.Renderer.Event(e)
}

func (l *logged) Done(s Summary) {
	l.append(s.Record())
	l.Renderer.Done(s)
}
