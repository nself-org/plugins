package planview

// Purpose: the error code of the inspection surface (Epic SCHED D17).
// Constraints: registered through model.Register from this package's init.

import "github.com/nself-org/plugins/free/ci/internal/model"

// CodeExplainTarget is E692: the explain target is unknown or has no decision record.
const CodeExplainTarget = "E692"

func init() {
	model.Register(model.Code{ID: CodeExplainTarget, Class: "usage", Summary: "explain target not found or no decision record", Fix: "Name a job of the current plan, or run `nself ci runs decisions` to list recorded placement decisions"})
}
