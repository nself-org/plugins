package scorer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nself-org/nself-eval-gate/internal/schema"
	"golang.org/x/sync/semaphore"
)

// EvalRunResult is the output of AggregateScorer.Run.
// Purpose: Typed result of running all tasks in a suite; written to np_eval_runs.
// Inputs: populated by AggregateScorer after fan-out task scoring.
// Outputs: returned to HTTP handler and CLI for display/storage.
// Constraints: PassRate and SuiteScore in [0,1]; Passed = SuiteScore >= suite threshold.
type EvalRunResult struct {
	PassRate   float64         `json:"pass_rate"`
	SuiteScore float64         `json:"suite_score"`
	Passed     bool            `json:"passed"`
	Results    []schema.TaskResult `json:"tasks"`
	DurationMS int             `json:"duration_ms"`
}

// AggregateScorer runs all tasks in a suite in parallel, collects results, and computes
// pass_rate and suite_score (weighted mean).
// Purpose: Orchestrate per-task scoring, enforce concurrency bounds, persist run to DB.
// Inputs: tasks slice (from DB or YAML), scorer factory, threshold, concurrency limit.
// Outputs: EvalRunResult with all per-task results and aggregate metrics.
// Constraints: Concurrency bounded by semaphore (NSELF_EVAL_GATE_MAX_CONCURRENCY).
//
//	DB InsertRun called exactly once per aggregate run.
type AggregateScorer struct {
	// MaxConcurrency bounds the number of parallel task-scoring goroutines.
	MaxConcurrency int64
	// Threshold is the suite-level gate threshold (suite_score >= threshold → passed).
	Threshold float64
}

// TaskScoreInput bundles a task with the scorer to use for it.
type TaskScoreInput struct {
	Task   schema.EvalTask
	Scorer Scorer
}

// Run executes all tasks using bounded concurrency and returns the aggregate result.
// Purpose: Fan-out scoring, collect results, compute metrics.
// Inputs: ctx, output (single model output used for all tasks), inputs (task+scorer pairs), suiteThreshold.
// Outputs: EvalRunResult or error if any task scoring panics.
// Constraints: Uses golang.org/x/sync/semaphore for bounded fan-out; DB write is caller's responsibility.
func (a *AggregateScorer) Run(ctx context.Context, output string, inputs []TaskScoreInput, suiteThreshold float64) (EvalRunResult, error) {
	start := time.Now()

	maxConcurrency := a.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = 4
	}

	sem := semaphore.NewWeighted(maxConcurrency)
	results := make([]schema.TaskResult, len(inputs))
	errs := make([]error, len(inputs))

	var wg sync.WaitGroup
	for i, inp := range inputs {
		wg.Add(1)
		go func(idx int, input TaskScoreInput) {
			defer wg.Done()
			if err := sem.Acquire(ctx, 1); err != nil {
				errs[idx] = fmt.Errorf("semaphore acquire for task %s: %w", input.Task.ID, err)
				return
			}
			defer sem.Release(1)

			scoreResult, err := input.Scorer.Score(ctx, output, input.Task)
			if err != nil {
				errs[idx] = fmt.Errorf("task %s score error: %w", input.Task.ID, err)
				results[idx] = schema.TaskResult{
					ID:      input.Task.ID,
					Score:   0.0,
					Metrics: map[string]float64{},
					Passed:  false,
				}
				return
			}

			results[idx] = schema.TaskResult{
				ID:        input.Task.ID,
				Score:     scoreResult.Score,
				Metrics:   scoreResult.Metrics,
				Passed:    scoreResult.Passed,
				Rationale: scoreResult.Rationale,
			}
		}(i, inp)
	}
	wg.Wait()

	// Check for context cancellation.
	if ctx.Err() != nil {
		return EvalRunResult{}, ctx.Err()
	}

	// Compute pass_rate and suite_score.
	var totalWeight, weightedScoreSum float64
	passing := 0

	for _, r := range results {
		// All tasks get equal weight for pass_rate; weighted mean uses score.
		totalWeight += 1.0
		weightedScoreSum += r.Score
		if r.Passed {
			passing++
		}
	}

	n := float64(len(results))
	passRate := 0.0
	suiteScore := 0.0
	if n > 0 {
		passRate = float64(passing) / n
		suiteScore = weightedScoreSum / totalWeight
	}

	passed := suiteScore >= suiteThreshold

	return EvalRunResult{
		PassRate:   passRate,
		SuiteScore: suiteScore,
		Passed:     passed,
		Results:    results,
		DurationMS: int(time.Since(start).Milliseconds()),
	}, nil
}
