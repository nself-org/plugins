package planview

// Purpose: the DecisionSource registry. `nself ci explain <run>/<job>` asks
// the registered source for a past placement; none is registered until
// P7-SCHED-05 adds the store-backed source.
// Inputs: Register(s). Outputs: Registered() and Lookup.
// Constraints: one source; a second Register replaces the first (tests restore
// it); Lookup with no source is E692 and names `nself ci runs decisions`.

import (
	"context"
	"fmt"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// DecisionSource returns the recorded placement of one job of one run as a
// placement-plan document holding that job alone (source decision-record).
// A missing run, job or record is an *Error with code E692; Lookup also
// refuses a document that is not placement-plan v1 or lacks the job asked for.
type DecisionSource interface {
	Decision(ctx context.Context, run, job string) (model.PlacementPlan, error)
}

// Error is a coded planview failure.
type Error struct{ Code, Message string }

func (e *Error) Error() string { return e.Code + ": " + e.Message }

var (
	mu     sync.RWMutex
	source DecisionSource
)

// Register installs the decision source (nil removes it).
func Register(s DecisionSource) { mu.Lock(); source = s; mu.Unlock() }

// Registered returns the installed source, or nil.
func Registered() DecisionSource { mu.RLock(); defer mu.RUnlock(); return source }

// Lookup asks the registered source for run/job.
func Lookup(ctx context.Context, run, job string) (model.PlacementPlan, error) {
	s := Registered()
	if s == nil {
		return model.PlacementPlan{}, &Error{CodeExplainTarget, fmt.Sprintf("no decision records are available for run %q; list recorded decisions with `nself ci runs decisions`", run)}
	}
	doc, err := s.Decision(ctx, run, job)
	if err != nil {
		return model.PlacementPlan{}, err
	}
	if doc.Schema != SchemaID || doc.Jobs == nil {
		return model.PlacementPlan{}, &Error{CodeExplainTarget, fmt.Sprintf("the decision record of run %q is not a %s document", run, SchemaID)}
	}
	one := Select(doc, func(j model.JobPlacement) bool { return j.ID == job || j.Name == job })
	if len(one.Jobs) == 0 {
		return model.PlacementPlan{}, &Error{CodeExplainTarget, fmt.Sprintf("run %q has no recorded decision for job %q; list recorded decisions with `nself ci runs decisions`", run, job)}
	}
	return one, nil
}
