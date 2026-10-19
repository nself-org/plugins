package run

// Purpose: the seam that lets a finished run's evidence carry flaky-by-history
// debt (P7-CI-40) without run importing the history reader.
// Inputs:  the store and the finished run's evidence.
// Outputs: a Flaky note per check id, attached to the check.
// Constraints: registered at init (cmd/runs_flaky.go); a nil hook, a panic or
// an empty answer leaves the evidence as it was; a marker never changes a
// result, a verdict or an exit code.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// FlakyHistoryHook answers which checks of ev were flaky across recent runs.
type FlakyHistoryHook func(ctx context.Context, st *store.Store, ev model.Evidence) map[string]*model.Flaky

// FlakyHistory is nil until the flaky detector registers.
var FlakyHistory FlakyHistoryHook

// MarkHistoryFlaky attaches the history notes to the checks that carry none.
// It is idempotent. An EvidenceBuilder calls it once the input digests are set
// and before it writes per-attempt documents (D-1050); the coordinator calls it
// again on the final run document.
func MarkHistoryFlaky(ctx context.Context, st *store.Store, ev *model.Evidence) {
	if FlakyHistory == nil {
		return
	}
	var notes map[string]*model.Flaky
	if guard("flaky history hook", func() error { notes = FlakyHistory(ctx, st, *ev); return nil }) != nil {
		return
	}
	for i := range ev.Checks {
		if n := notes[ev.Checks[i].ID]; n != nil && ev.Checks[i].Flaky == nil {
			ev.Checks[i].Flaky = n
		}
	}
}
