package burst

// Purpose: the sweeper's verdict on one listed machine (Epic D12), pure.
// Inputs:  SweepFacts: whether the machine carries this coordinator's labels, is
// in the exclusion set, has an intent row and in which state, and its age.
// Outputs: SweepVerdict{Delete, Reason}.
// Constraints: no os, os/exec, net or database/sql (TestDecideIsPure); a machine
// that is not this coordinator's, or is excluded, is never deleted; age only
// ever forces a deletion (a machine older than max_lifetime + grace goes, so a
// leak cannot hide behind a stuck intent) and never protects one; a live intent's
// machine younger than max_lifetime is never deleted; a terminal or tearing-down
// intent's machine always is.

// The sweep reasons.
const (
	SweepForeign        = "foreign"
	SweepExcluded       = "excluded"
	SweepIntentTerminal = "intent.terminal"
	SweepIntentTearing  = "intent.tearing_down"
	SweepAgeExpired     = "age.expired"
	SweepOrphanExpired  = "orphan.expired"
	SweepLive           = "live"
	SweepOrphan         = "orphan"
	sweepIntentTearing  = "tearing-down"
	sweepIntentTerminal = "terminal"
)

// SweepFacts are the inputs of SweepDecide.
type SweepFacts struct {
	// Mine: labelled role=ci-ephemeral with this coordinator's id. Excluded: in the NODE-14 set.
	Mine, Excluded bool
	// HasIntent and IntentState (live, tearing-down, terminal) describe the intent row named by the label.
	HasIntent   bool
	IntentState string
	// AgeMs is the machine's age by the provider's CreatedAt; MaxLifetimeMs and GraceMs the limits.
	AgeMs, MaxLifetimeMs, GraceMs int64
}

// SweepVerdict is the answer.
type SweepVerdict struct {
	Delete bool
	Reason string
}

// SweepDecide applies the rules in order.
func SweepDecide(f SweepFacts) SweepVerdict {
	switch {
	case !f.Mine:
		return SweepVerdict{false, SweepForeign}
	case f.Excluded:
		return SweepVerdict{false, SweepExcluded}
	case f.HasIntent && f.IntentState == sweepIntentTerminal:
		return SweepVerdict{true, SweepIntentTerminal}
	case f.HasIntent && f.IntentState == sweepIntentTearing:
		return SweepVerdict{true, SweepIntentTearing}
	case f.AgeMs > satAdd(max(f.MaxLifetimeMs, 0), max(f.GraceMs, 0)):
		if f.HasIntent {
			return SweepVerdict{true, SweepAgeExpired}
		}
		return SweepVerdict{true, SweepOrphanExpired}
	case f.HasIntent:
		return SweepVerdict{false, SweepLive}
	}
	return SweepVerdict{false, SweepOrphan}
}
