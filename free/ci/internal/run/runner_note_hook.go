package run

// Purpose: the runner that actually ran an attempt, kept for its evidence. The
// dispatcher knows the chosen runner when it admits an attempt; the document
// is built later, so the placement is noted here (as PlacementRef is).
// Inputs:  NoteRunner(attempt, runner) from internal/schedtel when it stores an
// admitted decision; ForgetRunner(attempt) when the run closes.
// Outputs: placedRunners(jobs): the per-attempt records and the run-level runner.
// Constraints: in-process and not persisted (the placement record itself is in
// the store); evidence is built before finalizers run, so a finalizer may forget.
// A run whose executed jobs all ran on one runner names that runner; any other
// run names the coordinator host (the producer) and lists every attempt.

import (
	"runtime"
	"sort"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

var placedRunnerNotes sync.Map // attempt id -> model.AttemptRunner

// NoteRunner records where the attempt was admitted. A runner with no node id is ignored.
func NoteRunner(attemptID string, r model.AttemptRunner) {
	if attemptID != "" && r.Node != "" {
		placedRunnerNotes.Store(attemptID, r)
	}
}

// ForgetRunner drops the note of a finished attempt.
func ForgetRunner(attemptID string) { placedRunnerNotes.Delete(attemptID) }

// RunnerNote returns the runner an attempt was admitted on, false when none was noted.
func RunnerNote(attemptID string) (model.AttemptRunner, bool) {
	v, ok := placedRunnerNotes.Load(attemptID)
	if !ok {
		return model.AttemptRunner{}, false
	}
	return v.(model.AttemptRunner), true
}

// RunnerRefOf is the schema's run-level runner for an attempt runner.
func RunnerRefOf(r model.AttemptRunner) model.RunnerRef {
	return model.RunnerRef{ID: r.Node, Provider: r.Provider, Platform: r.Platform, Isolation: r.Isolation}
}

// CoordinatorRunner is the host that produced the document: where a job runs when nothing says otherwise.
func CoordinatorRunner() model.RunnerRef {
	return model.RunnerRef{ID: "local", Provider: "local", Platform: runtime.GOOS + "/" + runtime.GOARCH, Isolation: "process"}
}

// placedRunners derives the attempt records and the run-level runner from the executed jobs.
// An executed job with no note ran on the coordinator host (the local path admits without a placement).
func placedRunners(jobs []JobOutcome) ([]model.AttemptEvidence, model.RunnerRef) {
	var attempts []model.AttemptEvidence
	byNode := map[string]model.RunnerRef{}
	for _, o := range jobs {
		if o.AttemptID == "" || o.State == StateSkipped {
			continue
		}
		r, ok := RunnerNote(o.AttemptID)
		if !ok {
			local := CoordinatorRunner()
			byNode[local.ID] = local
			continue
		}
		byNode[r.Node] = RunnerRefOf(r)
		attempts = append(attempts, model.AttemptEvidence{JobID: o.ID, Attempt: max(o.N, 1), Runner: r})
	}
	if len(byNode) != 1 {
		return attempts, CoordinatorRunner()
	}
	ids := make([]string, 0, 1)
	for id := range byNode {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return attempts, byNode[ids[0]]
}
