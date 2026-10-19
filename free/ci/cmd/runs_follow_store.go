package main

// Purpose: `nself ci runs follow` for a run with no event file whose local store
// holds its event log (D-1061): replay the stored records and tail the log while
// the run is live, so per-job progress is visible for a foreground run.
// Inputs:  the run source (local store) and the first seq to print.
// Outputs: NDJSON on stdout, the same shape as every other follow.
// Constraints: the log is always read from seq 1 so emitter seqs equal stored
// seqs (lines below --from-seq are dropped by the synth); a run with no log that
// is already over is rebuilt by the caller; the store marks a run over just
// before its engine writes the last record, so the log gets storedGrace to
// finish before the stream is closed from the run state.

import (
	"context"
	"os"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/detach"
	"github.com/nself-org/plugins/free/ci/internal/events"
)

// storedGrace is how long a finished run's log may stay without its `finished` record.
var storedGrace = 3 * time.Second

// followStored streams the stored log; ok is false when the run has none and is
// over, so the caller rebuilds it.
func followStored(ctx context.Context, src *runSource, v detach.View, from int64) (code int, ok bool) {
	syn := detach.NewSynth(os.Stdout, v.RunID, from)
	next, finished := int64(1), false
	var overAt time.Time
	for {
		rows, err := src.st.EventRows(ctx, v.RunID, next)
		if err != nil {
			syn.Flush()
			return followFail(err, "", 2), true
		}
		for _, row := range rows {
			rec, err := events.ParseRecord(row.Type, row.Ref)
			if err != nil {
				syn.Flush()
				return followFail(err, "", 2), true
			}
			syn.Replay(rec)
			next, finished = row.Seq+1, finished || rec.Type == "finished"
		}
		over := detach.Terminal(v.State)
		if over && next == 1 {
			return 0, false
		}
		if over && overAt.IsZero() {
			overAt = time.Now()
		}
		if finished || over && time.Since(overAt) >= storedGrace {
			syn.FinishStored(v, finished, evidenceOf(ctx, src, v))
			return 0, true
		}
		select {
		case <-ctx.Done():
			syn.Flush()
			return followFail(ctx.Err(), "", 1), true
		case <-time.After(waitPoll):
		}
		if v, err = src.View(ctx); err != nil {
			syn.Flush()
			return followFail(err, "", exitForError(err)), true
		}
	}
}
