package scorer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/nself-org/nself-eval-gate/internal/schema"
)

// SemanticScorer computes cosine similarity between expected and model output embeddings.
// Purpose: Quality scorer for tasks where meaning matters more than exact wording.
// Inputs: output (model response), task.ExpectedOutput (ground truth),
//
//	embedURL (plugin-retrieval embed endpoint), embedTimeout.
//
// Outputs: cosine similarity [0,1] as Score; ErrPreconditionNotMet if embed unavailable.
// Constraints: Depends on plugin-retrieval BGE-M3 (E3 precondition). Returns
//
//	ErrPreconditionNotMet on endpoint failure so CLI exits code 3, not code 1.
type SemanticScorer struct {
	// EmbedURL is the full URL to the plugin-retrieval embed endpoint.
	// Example: "http://localhost:3771/embed"
	EmbedURL string
	// Timeout is per-embed-call timeout.
	Timeout time.Duration
	// HTTPClient is the HTTP client used for embed calls (injectable for testing).
	HTTPClient *http.Client
}

// embedRequest is the JSON body sent to plugin-retrieval.
type embedRequest struct {
	Texts []string `json:"texts"`
	Model string   `json:"model"`
}

// embedResponse is the JSON response from plugin-retrieval.
type embedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
}

// Score computes cosine similarity between expected and model output via BGE-M3.
func (s *SemanticScorer) Score(ctx context.Context, output string, task schema.EvalTask) (ScoreResult, error) {
	client := s.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: s.Timeout}
	}

	reqBody, err := json.Marshal(embedRequest{
		Texts: []string{task.ExpectedOutput, output},
		Model: "bge-m3",
	})
	if err != nil {
		return ScoreResult{}, fmt.Errorf("semantic scorer marshal: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, s.EmbedURL, bytes.NewReader(reqBody))
	if err != nil {
		return ScoreResult{}, ErrPreconditionNotMet
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		// Treat connection errors as precondition failures — not quality regressions.
		return ScoreResult{}, fmt.Errorf("%w: embed endpoint unavailable: %v", ErrPreconditionNotMet, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ScoreResult{}, fmt.Errorf("%w: embed endpoint returned HTTP %d", ErrPreconditionNotMet, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ScoreResult{}, fmt.Errorf("%w: reading embed response: %v", ErrPreconditionNotMet, err)
	}

	var embedResp embedResponse
	if err := json.Unmarshal(body, &embedResp); err != nil {
		return ScoreResult{}, fmt.Errorf("%w: parsing embed response: %v", ErrPreconditionNotMet, err)
	}

	if len(embedResp.Embeddings) < 2 {
		return ScoreResult{}, fmt.Errorf("%w: expected 2 embeddings, got %d", ErrPreconditionNotMet, len(embedResp.Embeddings))
	}

	similarity := cosineSimilarity(embedResp.Embeddings[0], embedResp.Embeddings[1])

	return ScoreResult{
		Score:   similarity,
		Metrics: map[string]float64{"cosine_similarity": similarity},
		Passed:  similarity >= task.Threshold,
	}, nil
}

// cosineSimilarity computes the cosine similarity between two float64 vectors.
// Returns 0.0 for zero vectors to guard against divide-by-zero.
func cosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0.0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0.0 {
		// Zero vector — guard against divide-by-zero.
		return 0.0
	}

	sim := dot / denom
	// Clamp to [-1, 1] due to floating point drift, then map to [0, 1].
	if sim > 1.0 {
		sim = 1.0
	} else if sim < -1.0 {
		sim = -1.0
	}
	// BGE-M3 embeddings are normalized, so similarity is naturally [0,1].
	// Return raw value; callers can clamp if needed.
	return sim
}
