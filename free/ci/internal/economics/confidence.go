package economics

// Purpose: the four confidence labels of a ledger figure (Epic D11) and the
// rule that no figure leaves this package without one.
// Inputs:  none. Outputs: ConfKnown, ConfComputed, ConfEstimated, ConfUnknown.
// Constraints: `known` is for figures a provider reported (RecordUsage) and
// nothing else; `computed` is for local actuals from the coordinator's own
// timestamps; `estimated` is the bound at admission, a clamped remote actual,
// and every allowance without a provider balance; `unknown` is a figure with
// no basis (a duration nobody recorded).

// The confidence labels.
const (
	ConfKnown     = "known"
	ConfComputed  = "computed"
	ConfEstimated = "estimated"
	ConfUnknown   = "unknown"
	// ConfDeclared labels a value the operator declared in policy (a rate, a ceiling, a budget
	// limit): it is an input, not a measurement of spend.
	ConfDeclared = "declared"
)

// Figure is an amount that carries its confidence and where it came from.
type Figure struct {
	Micro      int64  `json:"micro"`
	Confidence string `json:"confidence"`
	Source     string `json:"source"`
}
