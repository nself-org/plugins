package background

// Purpose: the `background` admission step (Epic D14): inside the admission
// transaction, a background-band job is refused while an eligible interactive job
// is queued for the same runner capacity (background.interactive_queued), and on a
// node in contention (background.contention). It never preempts: a refusal leaves
// the job queued and a running attempt is never touched.
// Inputs:  the dispatch.View of one admission (runner, job band) and the optional
// QueueProbe that says whether an interactive job waits for a runner.
// Outputs: nil (admit) or a *dispatch.Refusal naming the reason.
// Constraints: this is a second look at the placement's own snapshot
// (Request.Runner is the runner the same tick's world gave placement), not a
// fresh read: the transaction re-reads node state, counters and reservations but
// not availability facts. The queue probe is the live input. Only the background
// band is judged, so a foreground or normal job keeps its slot on a battery node
// (X17, the foreground floor). The step reads no Store (the admission holds the writer). Reservations are enforced by admission
// itself, not here. With no probe installed the queue half admits and the
// placement stage (sched eligibility, fed by the world's InteractiveQueued)
// remains the guard; contention is always checked.

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// StepName is the registered name; steps run in name order.
const StepName = "background"

// QueueProbe reports whether an eligible interactive job is queued for the
// capacity of runnerID. It is called inside the admission transaction, so it
// must not touch the store, and it sees only the runner id: it must answer for
// interactive-band jobs alone.
type QueueProbe func(runnerID string) bool

var probe atomic.Pointer[QueueProbe]

// SetQueueProbe installs the interactive-queue source (nil removes it) and
// returns the previous one. The dispatcher wiring installs it once at start.
func SetQueueProbe(p QueueProbe) QueueProbe {
	var old *QueueProbe
	if p == nil {
		old = probe.Swap(nil)
	} else {
		old = probe.Swap(&p)
	}
	if old == nil {
		return nil
	}
	return *old
}

func init() {
	if err := dispatch.RegisterStep(StepName, Step); err != nil {
		panic(err)
	}
}

// Step is the admission step; see the file comment.
func Step(_ context.Context, v dispatch.View) error {
	if v.Request.Job.Pipeline.Band != BandName {
		return nil
	}
	r := v.Request.Runner
	if p := probe.Load(); p != nil && (*p)(r.ID) {
		return &dispatch.Refusal{Reason: model.BackgroundInteractiveQueued, Detail: "an interactive job is queued for runner " + r.ID}
	}
	if verdict := Contended(r); verdict.Busy() {
		causes := make([]string, len(verdict.Causes))
		for i, c := range verdict.Causes {
			causes[i] = string(c)
		}
		return &dispatch.Refusal{Reason: model.BackgroundContention, Detail: "runner " + r.ID + ": " + strings.Join(causes, "; ")}
	}
	return nil
}
