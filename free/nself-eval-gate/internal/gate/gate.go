// Package gate implements the autonomy-tier clearance check for nself-eval-gate.
// Answers the boolean question: "is this autonomy tier cleared?" for the CLI and HTTP layer.
package gate

import (
	"context"
	"fmt"

	"github.com/nself-org/nself-eval-gate/internal/db"
)

// TierResult holds the outcome of an IsTierCleared check.
// Purpose: Typed return from IsTierCleared for CLI exit code routing and HTTP response.
// Inputs: populated by IsTierCleared after evaluating all required suites.
// Outputs: returned by GET /eval/gate/{tier} and used in CLI exit code logic.
// Constraints: Cleared=false always has at least one BlockingSuite when enforced=true.
type TierResult struct {
	Tier           string   `json:"tier"`
	Cleared        bool     `json:"cleared"`
	BlockingSuites []string `json:"blocking_suites,omitempty"`
	Enforced       bool     `json:"enforced"`
}

// IsTierCleared checks whether all required eval suites for an autonomy tier have passed.
// Purpose: Core gate decision: check np_eval_thresholds + latest np_eval_runs per suite.
// Inputs: ctx, tier string (supervised|semi-auto|full-auto), store, sourceAccountID.
// Outputs: TierResult{Cleared, BlockingSuites} or error on DB failure.
// Constraints:
//   - supervised tier always returns Cleared=true (enforced=false in seed data).
//   - Gate uses AND semantics: BOTH min_pass_rate AND min_suite_score must clear.
//   - If no run exists for a required suite: that suite blocks the tier.
//   - Missing threshold record for tier: returns error (misconfiguration).
func IsTierCleared(ctx context.Context, tier string, store db.Store, sourceAccountID string) (TierResult, error) {
	threshold, err := store.GetThresholdByTier(ctx, tier)
	if err != nil {
		return TierResult{}, fmt.Errorf("IsTierCleared: fetch threshold for tier %q: %w", tier, err)
	}
	if threshold == nil {
		return TierResult{}, fmt.Errorf("IsTierCleared: no threshold configured for tier %q", tier)
	}

	// Supervised tier: enforced=false → always cleared.
	if !threshold.Enforced {
		return TierResult{
			Tier:     tier,
			Cleared:  true,
			Enforced: false,
		}, nil
	}

	// No suites required → cleared (defensive: applies_to is empty only for supervised normally).
	if len(threshold.AppliesTo) == 0 {
		return TierResult{
			Tier:     tier,
			Cleared:  true,
			Enforced: true,
		}, nil
	}

	var blockingSuites []string

	for _, slug := range threshold.AppliesTo {
		// Resolve suite by slug.
		suite, err := store.GetSuiteBySlug(ctx, slug, sourceAccountID)
		if err != nil {
			return TierResult{}, fmt.Errorf("IsTierCleared: look up suite %q: %w", slug, err)
		}
		if suite == nil {
			// Suite not registered yet → blocks.
			blockingSuites = append(blockingSuites, slug+" (not registered)")
			continue
		}

		// Get latest run for the suite.
		run, err := store.GetLatestRunBySuite(ctx, suite.ID, sourceAccountID)
		if err != nil {
			return TierResult{}, fmt.Errorf("IsTierCleared: get latest run for suite %q: %w", slug, err)
		}
		if run == nil {
			// No runs yet → blocks.
			blockingSuites = append(blockingSuites, slug+" (no runs)")
			continue
		}

		// AND semantics: both pass_rate AND suite_score must clear.
		passRateOK := run.PassRate >= threshold.MinPassRate
		suiteScoreOK := run.SuiteScore >= threshold.MinSuiteScore

		if !passRateOK || !suiteScoreOK {
			reason := fmt.Sprintf("%s (pass_rate=%.3f/%.3f suite_score=%.3f/%.3f)",
				slug, run.PassRate, threshold.MinPassRate, run.SuiteScore, threshold.MinSuiteScore)
			blockingSuites = append(blockingSuites, reason)
		}
	}

	cleared := len(blockingSuites) == 0
	return TierResult{
		Tier:           tier,
		Cleared:        cleared,
		BlockingSuites: blockingSuites,
		Enforced:       true,
	}, nil
}
