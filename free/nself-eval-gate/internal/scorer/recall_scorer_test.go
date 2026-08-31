// recall_scorer_test.go — golden-set regression tests for RecallScorer.
// Purpose: Verify fact_f1 >= 0.80 with synthetic data; verify ErrPreconditionNotMet when
//   plugin-retrieval is unavailable.
// Ref: eval-harness-foundation-spec.md §5b, §9.
package scorer_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nself-org/nself-eval-gate/internal/schema"
	"github.com/nself-org/nself-eval-gate/internal/scorer"
)

// TestRecallScorer_GoldenSet verifies fact_f1 >= 0.80 with a synthetic golden set
// where all golden triples are returned by the mock retrieval endpoint.
func TestRecallScorer_GoldenSet(t *testing.T) {
	t.Parallel()

	golden := []schema.GoldenMemory{
		{Subject: "alex", Predicate: "takes_medication", Object: "lisinopril 10mg daily"},
		{Subject: "alex", Predicate: "takes_medication", Object: "metformin 500mg twice daily"},
		{Subject: "alex", Predicate: "takes_medication", Object: "atorvastatin 20mg nightly"},
	}

	// Mock plugin-retrieval: return all three golden triples.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"memories": []map[string]interface{}{
				{"subject": "alex", "predicate": "takes_medication", "object": "lisinopril 10mg daily", "score": 0.95},
				{"subject": "alex", "predicate": "takes_medication", "object": "metformin 500mg twice daily", "score": 0.91},
				{"subject": "alex", "predicate": "takes_medication", "object": "atorvastatin 20mg nightly", "score": 0.88},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	s := &scorer.RecallScorer{
		RetrievalURL: srv.URL,
		K:            3,
		HTTPClient:   srv.Client(),
	}

	task := schema.EvalTask{
		ID:             "h001-test",
		Query:          "What medications does Alex take?",
		ScoringMode:    "recall",
		GoldenMemories: golden,
		Threshold:      0.80,
	}

	result, err := s.Score(context.Background(), "", task)
	if err != nil {
		t.Fatalf("Score() unexpected error: %v", err)
	}

	if result.Metrics["fact_f1"] < 0.80 {
		t.Errorf("fact_f1 = %.4f; want >= 0.80", result.Metrics["fact_f1"])
	}
	if !result.Passed {
		t.Errorf("Passed = false; expected true when fact_f1 >= 0.80")
	}

	// Verify all expected metrics are present.
	for _, key := range []string{"precision_at_k", "recall_at_k", "fact_f1"} {
		if _, ok := result.Metrics[key]; !ok {
			t.Errorf("missing metric %q in result.Metrics", key)
		}
	}
}

// TestRecallScorer_PartialRecall verifies fact_f1 is computed correctly when only
// a subset of golden triples are retrieved (tests the formula itself).
func TestRecallScorer_PartialRecall(t *testing.T) {
	t.Parallel()

	golden := []schema.GoldenMemory{
		{Subject: "alex", Predicate: "takes_medication", Object: "lisinopril 10mg daily"},
		{Subject: "alex", Predicate: "takes_medication", Object: "metformin 500mg twice daily"},
		{Subject: "alex", Predicate: "takes_medication", Object: "atorvastatin 20mg nightly"},
	}

	// Mock: return only 2 of 3 golden triples + 1 non-golden.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"memories": []map[string]interface{}{
				{"subject": "alex", "predicate": "takes_medication", "object": "lisinopril 10mg daily", "score": 0.95},
				{"subject": "alex", "predicate": "takes_medication", "object": "metformin 500mg twice daily", "score": 0.91},
				{"subject": "alex", "predicate": "prefers_food", "object": "pizza", "score": 0.50},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	s := &scorer.RecallScorer{
		RetrievalURL: srv.URL,
		K:            3,
		HTTPClient:   srv.Client(),
	}

	task := schema.EvalTask{
		Query:          "What medications does Alex take?",
		GoldenMemories: golden,
		Threshold:      0.80,
	}

	result, err := s.Score(context.Background(), "", task)
	if err != nil {
		t.Fatalf("Score() unexpected error: %v", err)
	}

	// precision@3 = 2/3 ≈ 0.667, recall@3 = 2/3 ≈ 0.667, f1 ≈ 0.667
	wantF1 := 2.0 * (2.0 / 3.0 * 2.0 / 3.0) / (2.0/3.0 + 2.0/3.0)
	got := result.Metrics["fact_f1"]
	if abs(got-wantF1) > 0.01 {
		t.Errorf("fact_f1 = %.4f; want ~%.4f", got, wantF1)
	}
}

// TestRecallScorer_PreconditionNotMet verifies that when plugin-retrieval is unavailable,
// Score() returns ErrPreconditionNotMet (not a generic error), so the CLI exits code 3.
func TestRecallScorer_PreconditionNotMet(t *testing.T) {
	t.Parallel()

	// Use a server that returns 503 to simulate plugin-retrieval being down.
	srv503 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"service unavailable"}`, http.StatusServiceUnavailable)
	}))
	defer srv503.Close()

	s := &scorer.RecallScorer{
		RetrievalURL: srv503.URL,
		K:            3,
		HTTPClient:   srv503.Client(),
	}

	task := schema.EvalTask{
		Query:          "What is Alex's health condition?",
		GoldenMemories: []schema.GoldenMemory{{Subject: "alex", Predicate: "has_condition", Object: "hypertension"}},
		Threshold:      0.80,
	}

	_, err := s.Score(context.Background(), "", task)
	if err == nil {
		t.Fatal("Score() should return error when plugin-retrieval is unavailable")
	}
	if !errors.Is(err, scorer.ErrPreconditionNotMet) {
		t.Errorf("Score() error should wrap ErrPreconditionNotMet; got: %v", err)
	}
}

// TestRecallScorer_NetworkDown verifies ErrPreconditionNotMet when the server is unreachable.
func TestRecallScorer_NetworkDown(t *testing.T) {
	t.Parallel()

	s := &scorer.RecallScorer{
		RetrievalURL: "http://127.0.0.1:19999", // nothing listening
		K:            3,
	}

	task := schema.EvalTask{
		Query:          "What are Alex's goals?",
		GoldenMemories: []schema.GoldenMemory{{Subject: "alex", Predicate: "has_goal", Object: "run a marathon"}},
		Threshold:      0.80,
	}

	_, err := s.Score(context.Background(), "", task)
	if err == nil {
		t.Fatal("Score() should return error on network failure")
	}
	if !errors.Is(err, scorer.ErrPreconditionNotMet) {
		t.Errorf("network error should wrap ErrPreconditionNotMet; got: %v", err)
	}
}

// TestRecallScorer_EmptyGolden verifies Score() returns 1.0 vacuously when no golden triples.
func TestRecallScorer_EmptyGolden(t *testing.T) {
	t.Parallel()

	s := &scorer.RecallScorer{RetrievalURL: "http://unused:9999", K: 3}

	task := schema.EvalTask{
		Query:          "anything",
		GoldenMemories: nil,
		Threshold:      0.80,
	}

	result, err := s.Score(context.Background(), "", task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Score != 1.0 {
		t.Errorf("Score = %.4f; want 1.0 for empty golden set", result.Score)
	}
	if !result.Passed {
		t.Errorf("Passed should be true for empty golden set")
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
