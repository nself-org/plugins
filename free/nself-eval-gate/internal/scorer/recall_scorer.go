// recall_scorer.go — Recall Quality scorer for ɳClaw memory retrieval regression testing.
// Purpose: Score a model's response against golden-set (subject, predicate, object) triples.
//   Computes precision@k, recall@k, and fact_f1 by querying plugin-retrieval BGE-M3 hybrid
//   search and extracting memory triples from the retrieved results.
// Inputs:  output (model response string), task.GoldenMemories (ground-truth triples),
//   RetrievalURL (plugin-retrieval /search endpoint), K (top-k results to retrieve).
// Outputs: ScoreResult{Score: fact_f1, Metrics: {precision_at_k, recall_at_k, fact_f1}, Passed: fact_f1 >= threshold}.
//   ErrPreconditionNotMet if plugin-retrieval is unavailable (so CLI exits code 3, not code 1).
// Constraints: fact_f1 = 2*(precision*recall)/(precision+recall). Triple match is case-insensitive
//   string equality on (subject, predicate, object). K defaults to 3 if 0.
// SPORT: F03-PLUGIN-REGISTRY.md nself-eval-gate recall gate wired
package scorer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nself-org/nself-eval-gate/internal/schema"
)

// RecallScorer scores recall-mode tasks by querying plugin-retrieval hybrid search and
// comparing retrieved memory triples against the golden set in the task spec.
// Purpose: BGE-M3 reranker regression detector for ɳClaw memory retrieval.
// Inputs:  RetrievalURL (plugin-retrieval base URL), K (top-k hits), HTTPClient (injectable).
// Outputs: ScoreResult{fact_f1}; ErrPreconditionNotMet if plugin-retrieval is down.
// Constraints: Returns ErrPreconditionNotMet on any HTTP error against plugin-retrieval
//   so CI exits code 3 (precondition failure) rather than code 1 (regression).
type RecallScorer struct {
	// RetrievalURL is the base URL of plugin-retrieval, e.g. "http://localhost:3771".
	RetrievalURL string
	// K is the number of top results to retrieve. Default 3 if 0.
	K int
	// Timeout is the per-request timeout. Default 10s if zero.
	Timeout time.Duration
	// HTTPClient is injectable for tests. Uses a default client if nil.
	HTTPClient *http.Client
}

// retrievalSearchRequest is the JSON body for POST /search on plugin-retrieval.
type retrievalSearchRequest struct {
	Query          string `json:"query"`
	K              int    `json:"k"`
	SourceAccount  string `json:"source_account_id,omitempty"`
}

// retrievalMemory is a single retrieved memory from plugin-retrieval.
type retrievalMemory struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Object    string `json:"object"`
	Score     float64 `json:"score"`
}

// retrievalSearchResponse is the JSON response from POST /search on plugin-retrieval.
type retrievalSearchResponse struct {
	Memories []retrievalMemory `json:"memories"`
}

// Score computes fact_f1 by querying plugin-retrieval for the task query and matching
// retrieved triples against task.GoldenMemories.
// Purpose: Run hybrid BGE-M3 search and measure precision/recall against golden set.
// Inputs:  ctx, output (ignored for recall mode), task (must have Query + GoldenMemories).
// Outputs: ScoreResult{Score: fact_f1}; ErrPreconditionNotMet if retrieval is unavailable.
// Constraints: If GoldenMemories is empty, returns Score=1.0 (vacuously true — no facts to check).
//   Triple matching is case-insensitive equality on (subject, predicate, object).
func (r *RecallScorer) Score(ctx context.Context, _ string, task schema.EvalTask) (ScoreResult, error) {
	k := r.K
	if k <= 0 {
		k = 3
	}

	golden := task.GoldenMemories
	if len(golden) == 0 {
		return ScoreResult{Score: 1.0, Passed: true, Metrics: map[string]float64{
			"precision_at_k": 1.0,
			"recall_at_k":    1.0,
			"fact_f1":        1.0,
		}}, nil
	}

	retrieved, err := r.search(ctx, task.Query, k)
	if err != nil {
		// Any retrieval failure is a precondition error, not a regression.
		return ScoreResult{}, fmt.Errorf("%w: plugin-retrieval search: %v", ErrPreconditionNotMet, err)
	}

	// Compute precision@k: fraction of retrieved triples that appear in the golden set.
	// Compute recall@k:    fraction of golden triples that appear in the retrieved set.
	truePositives := 0
	for _, mem := range retrieved {
		if matchesGolden(mem, golden) {
			truePositives++
		}
	}

	precisionAtK := 0.0
	if len(retrieved) > 0 {
		precisionAtK = float64(truePositives) / float64(len(retrieved))
	}

	recallAtK := 0.0
	if len(golden) > 0 {
		recallAtK = float64(truePositives) / float64(len(golden))
	}

	factF1 := 0.0
	if precisionAtK+recallAtK > 0 {
		factF1 = 2 * (precisionAtK * recallAtK) / (precisionAtK + recallAtK)
	}

	threshold := task.Threshold
	if threshold == 0 {
		threshold = 0.80
	}

	return ScoreResult{
		Score:  factF1,
		Passed: factF1 >= threshold,
		Metrics: map[string]float64{
			"precision_at_k": precisionAtK,
			"recall_at_k":    recallAtK,
			"fact_f1":        factF1,
		},
	}, nil
}

// search calls plugin-retrieval POST /search and returns retrieved memory triples.
func (r *RecallScorer) search(ctx context.Context, query string, k int) ([]retrievalMemory, error) {
	timeout := r.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	client := r.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	body, err := json.Marshal(retrievalSearchRequest{Query: query, K: k})
	if err != nil {
		return nil, fmt.Errorf("marshal search request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.RetrievalURL+"/search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		// Network error — treat as precondition failure (not a regression).
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
		return nil, errors.New("plugin-retrieval returned " + resp.Status)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plugin-retrieval returned %s", resp.Status)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result retrievalSearchResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result.Memories, nil
}

// matchesGolden returns true if mem matches any triple in golden (case-insensitive).
func matchesGolden(mem retrievalMemory, golden []schema.GoldenMemory) bool {
	ms := strings.ToLower(strings.TrimSpace(mem.Subject))
	mp := strings.ToLower(strings.TrimSpace(mem.Predicate))
	mo := strings.ToLower(strings.TrimSpace(mem.Object))
	for _, g := range golden {
		gs := strings.ToLower(strings.TrimSpace(g.Subject))
		gp := strings.ToLower(strings.TrimSpace(g.Predicate))
		go_ := strings.ToLower(strings.TrimSpace(g.Object))
		if ms == gs && mp == gp && mo == go_ {
			return true
		}
	}
	return false
}
