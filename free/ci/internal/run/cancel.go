package run

// Purpose: durable cancellation. `ci runs cancel <run>` (any process) writes
// a CAS cancel intent for every live attempt of the run; the coordinator that
// owns the run polls for intents and cancels the matching job, which kills its
// process tree. A cancel racing a completion has one winner: whichever commit
// reaches the store first (E605 for the loser).
// Inputs:  a run reference. Outputs: the number of attempts told to stop.
// Constraints: CancelRun never touches a terminal attempt; a stale epoch is
// re-read a few times because the coordinator moves attempts while we write.

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// CancelRun requests cancellation of every non-terminal attempt of a run.
func CancelRun(ctx context.Context, st *store.Store, ref, reason string) (int, error) {
	row, err := ResolveRun(ctx, st, ref)
	if err != nil {
		return 0, err
	}
	n := 0
	for round := 0; round < 5; round++ {
		attempts, err := st.PipelineAttempts(ctx, row.ID)
		if err != nil {
			return n, err
		}
		retry := false
		for _, a := range attempts {
			switch a.State {
			case "passed", "failed", "cancelled":
				continue
			}
			err = st.RequestCancel(ctx, a.ID, reason, a.Epoch)
			switch {
			case err == nil:
				n++
			case CodeOf(err) == codeStoreRef:
				retry = true // epoch moved, result committed, or already requested
			default:
				return n, err
			}
		}
		if !retry {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	return n, nil
}

// pollCancels watches for cancel intents until ctx is done.
func (rs *runState) pollCancels(ctx context.Context) {
	t := time.NewTicker(rs.sess.Poll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		ids, err := rs.sess.Store.PendingCancels(ctx, rs.sess.RunID)
		if err != nil || len(ids) == 0 {
			continue
		}
		rs.mu.Lock()
		var stop []func()
		for _, id := range ids {
			if job := rs.attempts[id]; job != "" {
				rs.userCancel = true
				if f := rs.cancels[job]; f != nil {
					stop = append(stop, f)
				}
			}
		}
		cancelRun := rs.cancelRun
		rs.mu.Unlock()
		for _, f := range stop {
			f()
		}
		if len(stop) > 0 && cancelRun != nil {
			cancelRun()
		}
	}
}
