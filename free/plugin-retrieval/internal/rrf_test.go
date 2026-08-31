package internal

import (
	"math"
	"testing"
)

// TestRRFMerge_BothLists verifies fusion of results appearing in both lists.
func TestRRFMerge_BothLists(t *testing.T) {
	vector := []DocumentResult{
		{ID: "a", Title: "Doc A"},
		{ID: "b", Title: "Doc B"},
	}
	text := []DocumentResult{
		{ID: "b", Title: "Doc B"},
		{ID: "c", Title: "Doc C"},
	}
	results := RRFMerge(vector, text, 10)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	// Doc B appears in both lists so should have the highest score
	if results[0].ID != "b" {
		t.Errorf("expected Doc B first (both lists), got %s", results[0].ID)
	}
}

// TestRRFMerge_ScoreFormula verifies 1/(k+rank) calculation for rank=0 in single list.
func TestRRFMerge_ScoreFormula(t *testing.T) {
	vector := []DocumentResult{{ID: "x", Title: "X"}}
	results := RRFMerge(vector, nil, 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result")
	}
	expected := 1.0 / (RRFConstant + 1.0)
	if math.Abs(results[0].RRFScore-expected) > 1e-9 {
		t.Errorf("expected RRF score %.9f, got %.9f", expected, results[0].RRFScore)
	}
}

// TestRRFMerge_TopK verifies the topK truncation.
func TestRRFMerge_TopK(t *testing.T) {
	docs := make([]DocumentResult, 10)
	for i := range docs {
		docs[i] = DocumentResult{ID: string(rune('a' + i)), Title: "Doc"}
	}
	results := RRFMerge(docs, nil, 3)
	if len(results) != 3 {
		t.Errorf("expected 3 results with topK=3, got %d", len(results))
	}
}

// TestRRFMerge_EmptyInputs handles nil/empty inputs gracefully.
func TestRRFMerge_EmptyInputs(t *testing.T) {
	results := RRFMerge(nil, nil, 10)
	if len(results) != 0 {
		t.Errorf("expected empty results for nil inputs, got %d", len(results))
	}
}

// TestRRFMerge_AdditiveScores verifies additive RRF scores for dual-list documents.
func TestRRFMerge_AdditiveScores(t *testing.T) {
	doc := DocumentResult{ID: "z", Title: "Z"}
	results := RRFMerge([]DocumentResult{doc}, []DocumentResult{doc}, 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result")
	}
	// Should be 2 * 1/(60+1) since rank=0 in both lists
	expected := 2.0 / (RRFConstant + 1.0)
	if math.Abs(results[0].RRFScore-expected) > 1e-9 {
		t.Errorf("additive score: expected %.9f, got %.9f", expected, results[0].RRFScore)
	}
}
