package logs

// Purpose: install the log store as the run loop's LogHook (P7-CI-39). Each
// attempt gets a Sink on <state>/logs/<run>/<attempt>/; the first attempt of
// a run triggers the retention prune (the "prune at run start").
// Inputs:  the run session and the attempt; AtRunStart, set by internal/history
// (logs cannot import history: history reads logs).
// Outputs: an AttemptLog the run loop writes the redacted output to.
// Constraints: no stored log is better than a failed job: any open error
// returns nil; ids are validated before they name a path; the prune runs once
// per run and never blocks on errors.

import (
	"context"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/run"
)

// AtRunStart runs retention once per run, before its first attempt log opens.
// history registers it from init(); nil means no pruning.
var AtRunStart func(ctx context.Context, s *run.Session)

func init() { run.RegisterLogs(&hook{}) }

type hook struct {
	mu   sync.Mutex
	last string // the run whose start was seen last
}

func (h *hook) OpenAttempt(ctx context.Context, s *run.Session, a run.AttemptInfo) run.AttemptLog {
	if s == nil || !ValidID(a.RunID) || !ValidID(a.AttemptID) {
		return nil
	}
	home := s.StateDir()
	if home == "" {
		return nil
	}
	h.mu.Lock()
	first := h.last != a.RunID
	h.last = a.RunID
	h.mu.Unlock()
	if fn := AtRunStart; fn != nil && first {
		fn(ctx, s)
	}
	w, err := Open(AttemptDir(home, a.RunID, a.AttemptID), Options{})
	if err != nil {
		return nil
	}
	return NewSink(w, "out")
}
