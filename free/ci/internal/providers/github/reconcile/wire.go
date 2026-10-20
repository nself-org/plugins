package reconcile

// Purpose: the process-wide Engine and its registration: the three policy keys and the two Reconcilers.
// Inputs:  init(); the coordinator start calls Use and UsePolicy once.
// Outputs: Default; RegistrationErrors lists what could not be registered.
// Constraints: nothing runs until the coordinator starts its Reconcilers and compat.V15() is true; with no
//          store installed (Use not called) a pass fails loudly instead of sweeping without marks.

import (
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/policy"
)

var def = NewEngine()

// Default returns the process-wide Engine the init() registered.
func Default() *Engine { return def }

// Use installs the store (the coordinator's *store.Store) on the default Engine.
func Use(s State) { def.Use(s) }

func init() {
	for _, k := range Keys() {
		if err := policy.RegisterKey(k); err != nil {
			registrationErrors = append(registrationErrors, err)
		}
	}
	for _, r := range []coordinator.Reconciler{def.GitHub(), def.Demand()} {
		if err := coordinator.RegisterReconciler(r); err != nil {
			registrationErrors = append(registrationErrors, err)
		}
	}
}
