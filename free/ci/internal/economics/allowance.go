package economics

// Purpose: the allowance of a provider (Epic D11): the owner-declared ceiling
// minus what the provider reported consuming and what the local ledger spent
// since. It is `estimated` unless the provider reports a balance.
// Inputs:  the spend policy and store.AllowanceInputs of a provider and month.
// Outputs: Allowance (remaining, whether consumption and ceiling are known, the confidence).
// Constraints: unknown consumption or no declared ceiling means unknown (the
// `included` rule then makes private hosted capacity ineligible, allowance.unknown);
// a reported balance replaces ceiling and imported consumption; all arithmetic saturates.

import (
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Allowance is what a provider still has for private work, with how sure we are.
type Allowance struct {
	Remaining    int64
	Known        bool // consumption is known: imported rows or a reported balance exist
	CeilingKnown bool // a ceiling is declared, or the provider reports a balance
	Confidence   string
	Source       string
}

// AllowanceOf computes the allowance of provider from its stored inputs.
func AllowanceOf(p Policy, provider string, in store.AllowanceInputs) Allowance {
	if in.Balance != nil {
		rem, _ := sched.Add(*in.Balance, -in.Local)
		return Allowance{Remaining: rem, Known: true, CeilingKnown: true, Confidence: ConfKnown, Source: "provider balance minus local ledger since the report"}
	}
	ceiling, declared := p.CeilingsMicro[provider]
	a := Allowance{Known: in.HasImport, CeilingKnown: declared, Confidence: ConfEstimated, Source: "declared ceiling minus imported consumption minus local ledger"}
	if !declared {
		a.Source, a.Confidence = "no ceiling declared", ConfUnknown
		return a
	}
	rem, _ := sched.Add(ceiling, -in.Imported)
	a.Remaining, _ = sched.Add(rem, -in.Local)
	if !in.HasImport {
		a.Source = "declared ceiling; no provider consumption imported"
	}
	return a
}

// ToSched is the scheduler's view of an allowance.
func (a Allowance) ToSched(margin int64) sched.Allowance {
	return sched.Allowance{RemainingEstimated: a.Remaining, MarginMicro: margin, Known: a.Known, CeilingKnown: a.CeilingKnown}
}
