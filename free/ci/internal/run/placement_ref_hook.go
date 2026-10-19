package run

// Purpose: sets the PlacementRef hook (P7-CI-38) so evidence carries the
// placement record of every attempt the dispatcher placed.
// Inputs:  NotePlacement(attempt, ref) from internal/schedtel when it stores a
// decision; ForgetPlacement(attempt) when the run closes.
// Outputs: PlacementRef(ctx, attempt) -> "sched:decision/<id>", false when the
// attempt was not placed by the dispatcher (v1.4 mode, or no record).
// Constraints: in-process and not persisted (the record itself is in the
// store); evidence is built before finalizers run, so a finalizer may forget.

import (
	"context"
	"sync"
)

var placementRefs sync.Map // attempt id -> ref

func init() { PlacementRef = notedPlacementRef }

// NotePlacement records the ref the attempt's evidence carries.
func NotePlacement(attemptID, ref string) {
	if attemptID != "" && ref != "" {
		placementRefs.Store(attemptID, ref)
	}
}

// ForgetPlacement drops the ref of a finished attempt.
func ForgetPlacement(attemptID string) { placementRefs.Delete(attemptID) }

func notedPlacementRef(_ context.Context, attemptID string) (string, bool) {
	v, ok := placementRefs.Load(attemptID)
	if !ok {
		return "", false
	}
	return v.(string), true
}
