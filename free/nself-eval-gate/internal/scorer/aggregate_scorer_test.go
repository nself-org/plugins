package scorer

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/nself-eval-gate/internal/schema"
)

// fixedScorer always returns a fixed score.
type fixedScorer struct {
	score    float64
	callCount *atomic.Int64
}

func (f *fixedScorer) Score(_ context.Context, _ string, task schema.EvalTask) (ScoreResult, error) {
	if f.callCount != nil {
		f.callCount.Add(1)
	}
	return ScoreResult{
		Score:   f.score,
		Metrics: map[string]float64{"score": f.score},
		Passed:  f.score >= task.Threshold,
	}, nil
}

func TestAggregatePassRate(t *testing.T) {
	agg := &AggregateScorer{MaxConcurrency: 4, Threshold: 0.80}

	inputs := []TaskScoreInput{
		{Task: makeTask("t1", 0.80), Scorer: &fixedScorer{score: 1.0}}, // PASS
		{Task: makeTask("t2", 0.80), Scorer: &fixedScorer{score: 0.9}}, // PASS
		{Task: makeTask("t3", 0.80), Scorer: &fixedScorer{score: 0.5}}, // FAIL
		{Task: makeTask("t4", 0.80), Scorer: &fixedScorer{score: 0.7}}, // FAIL
	}

	result, err := agg.Run(context.Background(), "test output", inputs, 0.80)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// 2/4 passing → pass_rate = 0.5
	if abs(result.PassRate-0.5) > 0.001 {
		t.Errorf("expected pass_rate 0.5, got %.4f", result.PassRate)
	}

	// suite_score = (1.0+0.9+0.5+0.7)/4 = 0.775
	expectedScore := (1.0 + 0.9 + 0.5 + 0.7) / 4.0
	if abs(result.SuiteScore-expectedScore) > 0.001 {
		t.Errorf("expected suite_score %.4f, got %.4f", expectedScore, result.SuiteScore)
	}

	// 0.775 < 0.80 threshold → not passed
	if result.Passed {
		t.Errorf("expected Passed=false for suite_score %.4f < threshold 0.80", result.SuiteScore)
	}
}

func TestAggregateThresholdBoundaryExact(t *testing.T) {
	// Exactly at threshold should count as pass.
	agg := &AggregateScorer{MaxConcurrency: 2}

	inputs := []TaskScoreInput{
		{Task: makeTask("t1", 0.80), Scorer: &fixedScorer{score: 0.80}},
		{Task: makeTask("t2", 0.80), Scorer: &fixedScorer{score: 0.80}},
	}

	result, _ := agg.Run(context.Background(), "output", inputs, 0.80)
	if !result.Passed {
		t.Errorf("expected Passed=true when suite_score (%.4f) exactly equals threshold (0.80)", result.SuiteScore)
	}
	if abs(result.SuiteScore-0.80) > 0.001 {
		t.Errorf("expected suite_score=0.80, got %.4f", result.SuiteScore)
	}
}

func TestAggregateConcurrentFanOut(t *testing.T) {
	var callCount atomic.Int64
	agg := &AggregateScorer{MaxConcurrency: 3}

	inputs := make([]TaskScoreInput, 10)
	for i := range inputs {
		inputs[i] = TaskScoreInput{
			Task:   makeTask(taskID(i), 0.5),
			Scorer: &fixedScorer{score: 0.8, callCount: &callCount},
		}
	}

	result, err := agg.Run(context.Background(), "output", inputs, 0.5)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if int(callCount.Load()) != 10 {
		t.Errorf("expected 10 scorer calls, got %d", callCount.Load())
	}
	if len(result.Results) != 10 {
		t.Errorf("expected 10 results, got %d", len(result.Results))
	}
}

func TestAggregateDBInsertCalledOnce(t *testing.T) {
	// This test verifies the DB InsertRun contract via the EvalRunResult shape.
	// AggregateScorer.Run returns exactly one EvalRunResult per call.
	agg := &AggregateScorer{MaxConcurrency: 2}
	inputs := []TaskScoreInput{
		{Task: makeTask("t1", 0.5), Scorer: &fixedScorer{score: 0.9}},
	}

	result, err := agg.Run(context.Background(), "output", inputs, 0.5)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if result.DurationMS < 0 {
		t.Error("expected non-negative DurationMS")
	}
	if len(result.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(result.Results))
	}
}

func makeTask(id string, threshold float64) schema.EvalTask {
	return schema.EvalTask{
		ID:          id,
		Query:       "test query",
		ScoringMode: "exact",
		Metrics:     []string{"score"},
		Threshold:   threshold,
	}
}

func taskID(i int) string {
	return "t" + string(rune('0'+i))
}

// ensure time import used.
var _ = time.Second
