// Package scorer provides the unified scoring interface and implementations for nself-eval-gate.
// Three scoring modes: exact-match, semantic (BGE-M3 cosine similarity), rubric (LLM-as-judge).
package scorer

import (
	"context"
	"errors"

	"github.com/nself-org/nself-eval-gate/internal/schema"
)

// ErrPreconditionNotMet is a sentinel error for infrastructure precondition failures.
// Purpose: Distinguish infrastructure issues (BGE-M3 not wired, gateway down) from
// quality regressions. CLI exits code 3 (not code 1) when this error propagates.
// Constraints: Must never be wrapped to lose sentinel identity; use errors.Is() to check.
var ErrPreconditionNotMet = errors.New("scoring precondition not met")

// Scorer is the unified interface all scoring modes implement.
// Purpose: Decouple aggregate runner from specific scoring strategies.
// Inputs: ctx for cancellation/timeout; output is the model's response string;
//
//	task carries the eval task spec including expected output, rubric, and mode.
//
// Outputs: ScoreResult with Score [0,1], per-metric breakdown, pass status.
// Constraints: Score == 0.0 + ErrPreconditionNotMet means infra failure (not regression).
type Scorer interface {
	Score(ctx context.Context, output string, task schema.EvalTask) (ScoreResult, error)
}

// ScoreResult holds the outcome of scoring one task.
// Purpose: Typed output from each Scorer implementation; aggregated by AggregateScorer.
// Inputs: populated by ExactScorer, SemanticScorer, or RubricScorer.
// Outputs: stored per-task in EvalRun.Results; surfaced in CLI output and /eval/runs/{id}.
// Constraints: Score in [0,1]; Passed = Score >= task.Threshold.
type ScoreResult struct {
	// Score is the normalized [0,1] float quality score for this task.
	Score float64 `json:"score"`
	// Metrics holds per-metric breakdown (e.g. precision_at_3, recall_at_3).
	Metrics map[string]float64 `json:"metrics"`
	// Rationale is the LLM judge's explanation (rubric mode only; empty otherwise).
	Rationale string `json:"rationale,omitempty"`
	// Passed indicates Score >= task.Threshold.
	Passed bool `json:"passed"`
}
