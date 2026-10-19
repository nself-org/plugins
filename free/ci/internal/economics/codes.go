package economics

// Purpose: the one error code of this package (Epic D17): E695, a spend that is
// refused. Every refusal carries the placement reason the scheduler uses
// (`budget.exceeded`, `paid.disabled`, `approval.pending` ...), so `ci explain`
// and the code agree.
// Inputs: a reason and a detail. Outputs: *SpendError, an error whose text starts with E695.
// Constraints: registered through model.Register, never a panic; a detail never
// holds a rate, a token or a secret, only amounts, scopes and ids.

import (
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeSpendRefused is the code of a refused spend.
const CodeSpendRefused = "E695"

func init() {
	model.Register(model.Code{ID: CodeSpendRefused, Class: "usage", Summary: "spend refused: no budget, paid placement disabled, or approval required",
		Fix: "Set economics.paid_enabled and a day or month budget, raise the budget, or approve the spend with `nself ci runs approve <run> --spend`"})
}

// SpendError is a refused spend and the placement reason that names it.
type SpendError struct {
	Reason model.PlacementReason
	Detail string
}

func (e *SpendError) Error() string {
	return fmt.Sprintf("%s: %s: %s", CodeSpendRefused, e.Reason, e.Detail)
}

func refuse(reason model.PlacementReason, format string, a ...any) *SpendError {
	return &SpendError{Reason: reason, Detail: fmt.Sprintf(format, a...)}
}
