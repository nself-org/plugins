package scorer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nself-org/nself-eval-gate/internal/cache"
	"github.com/nself-org/nself-eval-gate/internal/schema"
)

// countingCache counts Set calls to verify cache miss triggers gateway call.
type countingCache struct {
	setCalls int
	store    map[string][]byte
}

func (c *countingCache) Get(_ context.Context, key string) ([]byte, error) {
	if v, ok := c.store[key]; ok {
		return v, nil
	}
	return nil, cache.ErrCacheMiss
}

func (c *countingCache) Set(_ context.Context, key string, val []byte, _ time.Duration) error {
	c.setCalls++
	if c.store == nil {
		c.store = make(map[string][]byte)
	}
	c.store[key] = val
	return nil
}

func mockGatewayServer(t *testing.T, criterionScores []criterionScore) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		judgeJSON, _ := json.Marshal(judgeResponse{
			CriterionScores: criterionScores,
			Overall:         0.85,
		})
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": string(judgeJSON)}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

func TestRubricScorerWeightedMean(t *testing.T) {
	criteria := []criterionScore{
		{Name: "accuracy", Score: 0.9, Rationale: "mostly correct"},
		{Name: "completeness", Score: 0.7, Rationale: "missing one point"},
	}
	srv := mockGatewayServer(t, criteria)
	defer srv.Close()

	cc := &countingCache{store: make(map[string][]byte)}
	scorer := &RubricScorer{
		GatewayURL: srv.URL,
		Model:      "test-model",
		Timeout:    5 * time.Second,
		Cache:      cc,
	}

	task := schema.EvalTask{
		ID:          "t-rubric",
		ScoringMode: "rubric",
		Query:       "test query",
		Metrics:     []string{"accuracy", "completeness"},
		Threshold:   0.75,
		Rubric: &schema.Rubric{
			Criteria: []schema.RubricCriteria{
				{Name: "accuracy", Weight: 0.6, Description: "factual accuracy"},
				{Name: "completeness", Weight: 0.4, Description: "covers all points"},
			},
		},
	}

	result, err := scorer.Score(context.Background(), "test output", task)
	if err != nil {
		t.Fatalf("Score error: %v", err)
	}

	// weighted mean: (0.9*0.6 + 0.7*0.4) / (0.6+0.4) = (0.54+0.28)/1.0 = 0.82
	expectedScore := (0.9*0.6 + 0.7*0.4) / (0.6 + 0.4)
	if abs(result.Score-expectedScore) > 0.001 {
		t.Errorf("expected score %.4f, got %.4f", expectedScore, result.Score)
	}
	if !result.Passed {
		t.Errorf("expected Passed=true for score %.4f >= threshold 0.75", result.Score)
	}
}

func TestRubricScorerCacheHitSkipsGateway(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		criteria := []criterionScore{{Name: "accuracy", Score: 0.9, Rationale: "good"}}
		judgeJSON, _ := json.Marshal(judgeResponse{CriterionScores: criteria, Overall: 0.9})
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": string(judgeJSON)}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cc := &countingCache{store: make(map[string][]byte)}
	scorer := &RubricScorer{
		GatewayURL: srv.URL,
		Model:      "test-model",
		Timeout:    5 * time.Second,
		Cache:      cc,
	}
	task := schema.EvalTask{
		ID: "t-cache", ScoringMode: "rubric", Query: "q",
		Metrics: []string{"accuracy"}, Threshold: 0.5,
		Rubric: &schema.Rubric{
			Criteria: []schema.RubricCriteria{{Name: "accuracy", Weight: 1.0, Description: "test"}},
		},
	}

	// First call — should hit gateway.
	_, err := scorer.Score(context.Background(), "my output", task)
	if err != nil {
		t.Fatalf("first Score error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 gateway call on first Score, got %d", callCount)
	}

	// Second call with same output — should hit cache, not gateway.
	_, err = scorer.Score(context.Background(), "my output", task)
	if err != nil {
		t.Fatalf("second Score error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 total gateway call (cache hit on second), got %d", callCount)
	}
}

func TestRubricScorerCacheMissTriggersGateway(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		criteria := []criterionScore{{Name: "accuracy", Score: 0.8, Rationale: "ok"}}
		judgeJSON, _ := json.Marshal(judgeResponse{CriterionScores: criteria, Overall: 0.8})
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": string(judgeJSON)}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cc := &countingCache{store: make(map[string][]byte)}
	scorer := &RubricScorer{
		GatewayURL: srv.URL,
		Model:      "test-model",
		Timeout:    5 * time.Second,
		Cache:      cc,
	}
	task := schema.EvalTask{
		ID: "t-miss", ScoringMode: "rubric", Query: "q",
		Metrics: []string{"accuracy"}, Threshold: 0.5,
		Rubric: &schema.Rubric{
			Criteria: []schema.RubricCriteria{{Name: "accuracy", Weight: 1.0, Description: "test"}},
		},
	}

	// Different outputs → different cache keys → two gateway calls.
	_, _ = scorer.Score(context.Background(), "output one", task)
	_, _ = scorer.Score(context.Background(), "output two", task)
	if callCount != 2 {
		t.Errorf("expected 2 gateway calls for different outputs, got %d", callCount)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
