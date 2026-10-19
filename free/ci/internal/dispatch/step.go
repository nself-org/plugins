package dispatch

// Purpose: the admission step registry (Epic D7). A step runs inside the
// admission transaction, in name order, and refuses by returning an error; the
// transaction then rolls back and the job stays queued with the step's reason.
// Inputs:  RegisterStep calls from init() (limits here, background 12, budget 07,
// ephemeral 08, release CACHE-05, approval TRUST-24).
// Outputs: the ordered list Admit passes to the store.
// Constraints: names are unique and non-empty; registering after the first Admit
// is an error, so the order every admission sees is fixed for the process.

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// View is what a step sees: the store's in-transaction view and the request.
type View struct {
	store.AdmitView
	Request Request
}

// Step is one admission rule.
type Step func(context.Context, View) error

// Refusal is a step's answer "not now": the job stays queued with Reason.
type Refusal struct {
	Reason model.PlacementReason
	Detail string
}

func (r *Refusal) Error() string { return string(r.Reason) + ": " + r.Detail }

type namedStep struct {
	name string
	step Step
}

var (
	stepMu sync.Mutex
	steps  = map[string]Step{}
	sealed bool
	sorted []namedStep
)

// RegisterStep adds an admission step; it is an error after the first Admit.
func RegisterStep(name string, s Step) error {
	if name == "" || s == nil {
		return fmt.Errorf("dispatch: a step needs a name and a function")
	}
	stepMu.Lock()
	defer stepMu.Unlock()
	if sealed {
		return fmt.Errorf("dispatch: step %q registered after the first Admit", name)
	}
	if _, dup := steps[name]; dup {
		return fmt.Errorf("dispatch: step %q is already registered", name)
	}
	steps[name] = s
	return nil
}

// seal fixes the registry and returns the steps in name order.
func seal() []namedStep {
	stepMu.Lock()
	defer stepMu.Unlock()
	if !sealed {
		sealed = true
		for name, s := range steps {
			sorted = append(sorted, namedStep{name, s})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].name < sorted[j].name })
	}
	return sorted
}

// resetSteps is for tests in this package: it reopens registration.
func resetSteps(keep ...string) {
	stepMu.Lock()
	defer stepMu.Unlock()
	kept := map[string]Step{}
	for _, k := range keep {
		if s, ok := steps[k]; ok {
			kept[k] = s
		}
	}
	steps, sealed, sorted = kept, false, nil
}
