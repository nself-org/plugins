package scorer

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nself-org/nself-eval-gate/internal/schema"
)

func TestSemanticScorerCosineMath(t *testing.T) {
	// Known embeddings — [1,0] and [1,0] → cosine = 1.0
	srv := mockEmbedServer(t, [][]float64{{1.0, 0.0}, {1.0, 0.0}})
	defer srv.Close()

	scorer := &SemanticScorer{
		EmbedURL: srv.URL,
		Timeout:  5 * time.Second,
	}
	task := schema.EvalTask{
		ID: "t-semantic", Query: "q", ScoringMode: "semantic",
		ExpectedOutput: "same text", Metrics: []string{"cosine_similarity"},
		Threshold: 0.8,
	}
	result, err := scorer.Score(context.Background(), "same text", task)
	if err != nil {
		t.Fatalf("Score error: %v", err)
	}
	if math.Abs(result.Score-1.0) > 0.001 {
		t.Errorf("expected cosine ~1.0, got %.4f", result.Score)
	}
	if !result.Passed {
		t.Error("expected Passed=true for cosine=1.0 >= threshold=0.8")
	}
}

func TestSemanticScorerOrthogonalVectors(t *testing.T) {
	// [1,0] and [0,1] → cosine = 0.0
	srv := mockEmbedServer(t, [][]float64{{1.0, 0.0}, {0.0, 1.0}})
	defer srv.Close()

	scorer := &SemanticScorer{EmbedURL: srv.URL, Timeout: 5 * time.Second}
	task := schema.EvalTask{
		ID: "t-ortho", ScoringMode: "semantic",
		ExpectedOutput: "a", Metrics: []string{"cosine_similarity"},
		Threshold: 0.5,
	}
	result, _ := scorer.Score(context.Background(), "b", task)
	if result.Score > 0.001 {
		t.Errorf("expected cosine ~0.0 for orthogonal vectors, got %.4f", result.Score)
	}
	if result.Passed {
		t.Error("expected Passed=false for cosine=0.0 < threshold=0.5")
	}
}

func TestSemanticScorerErrPreconditionNotMet(t *testing.T) {
	// Mock server returning 503
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	scorer := &SemanticScorer{EmbedURL: srv.URL, Timeout: 5 * time.Second}
	task := schema.EvalTask{
		ID: "t-503", ScoringMode: "semantic",
		ExpectedOutput: "test", Metrics: []string{"cosine_similarity"},
		Threshold: 0.5,
	}
	_, err := scorer.Score(context.Background(), "test", task)
	if err == nil {
		t.Fatal("expected error for 503 response, got nil")
	}
	if !errors.Is(err, ErrPreconditionNotMet) {
		t.Errorf("expected ErrPreconditionNotMet, got %v", err)
	}
}

func TestSemanticScorerEndpointDown(t *testing.T) {
	// Point at a port that won't respond.
	scorer := &SemanticScorer{
		EmbedURL: "http://127.0.0.1:19999/embed",
		Timeout:  200 * time.Millisecond,
	}
	task := schema.EvalTask{
		ID: "t-down", ScoringMode: "semantic",
		ExpectedOutput: "test", Metrics: []string{"cosine_similarity"},
		Threshold: 0.5,
	}
	_, err := scorer.Score(context.Background(), "test", task)
	if err == nil {
		t.Fatal("expected error for unreachable endpoint, got nil")
	}
	if !errors.Is(err, ErrPreconditionNotMet) {
		t.Errorf("expected ErrPreconditionNotMet, got %v", err)
	}
}

func TestCosineSimilarityZeroVector(t *testing.T) {
	result := cosineSimilarity([]float64{0, 0, 0}, []float64{1, 2, 3})
	if result != 0.0 {
		t.Errorf("expected 0.0 for zero vector, got %.4f", result)
	}
}

func TestCosineSimilarityEmptyVectors(t *testing.T) {
	result := cosineSimilarity([]float64{}, []float64{})
	if result != 0.0 {
		t.Errorf("expected 0.0 for empty vectors, got %.4f", result)
	}
}

// mockEmbedServer returns a test HTTP server responding with the given embeddings.
func mockEmbedServer(t *testing.T, embeddings [][]float64) *httptest.Server {
	t.Helper()
	type respBody struct {
		Embeddings [][]float64 `json:"embeddings"`
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(respBody{Embeddings: embeddings})
	}))
}
