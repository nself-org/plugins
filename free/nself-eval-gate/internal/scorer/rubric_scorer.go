package scorer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/nself-org/nself-eval-gate/internal/cache"
	"github.com/nself-org/nself-eval-gate/internal/schema"
)

// RubricScorer uses nself-ai-gateway (OpenAI-compatible) as an LLM judge.
// Purpose: Score open-ended outputs against structured rubric criteria.
// Inputs: output (model response), task.Rubric (criteria with weights), cache for deduplication.
// Outputs: weighted mean of criterion scores [0,1]; cached by SHA256(output+rubric).
// Constraints: Calls nself-ai-gateway:3761 /v1/chat/completions; results cached 1h;
//
//	cache hit skips gateway call entirely; zero-score results are NOT cached.
type RubricScorer struct {
	// GatewayURL is the base URL for nself-ai-gateway (default http://localhost:3761).
	GatewayURL string
	// Model is the judge model ID (from NSELF_EVAL_GATE_JUDGE_MODEL env).
	Model string
	// Timeout is per-judge-call timeout.
	Timeout time.Duration
	// Cache is the eval cache for deduplication of judge results.
	Cache cache.EvalCache
	// HTTPClient is injectable for testing.
	HTTPClient *http.Client
}

// judgeRequestMessage is a single message in the OpenAI-compatible chat request.
type judgeRequestMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// judgeRequest is the OpenAI-compatible chat completions request body.
type judgeRequest struct {
	Model          string                `json:"model"`
	Messages       []judgeRequestMessage `json:"messages"`
	ResponseFormat *judgeResponseFormat  `json:"response_format,omitempty"`
	MaxTokens      int                   `json:"max_tokens,omitempty"`
}

// judgeResponseFormat enables structured JSON output from the gateway.
type judgeResponseFormat struct {
	Type string `json:"type"`
}

// judgeResponse is the structured JSON response expected from the LLM judge.
type judgeResponse struct {
	CriterionScores []criterionScore `json:"criterion_scores"`
	Overall         float64          `json:"overall"`
}

// criterionScore holds a single criterion's judge score and rationale.
type criterionScore struct {
	Name      string  `json:"name"`
	Score     float64 `json:"score"`
	Rationale string  `json:"rationale"`
}

// Score calls the LLM judge (via nself-ai-gateway) and returns a weighted mean rubric score.
func (r *RubricScorer) Score(ctx context.Context, output string, task schema.EvalTask) (ScoreResult, error) {
	if task.Rubric == nil || len(task.Rubric.Criteria) == 0 {
		return ScoreResult{}, fmt.Errorf("rubric scorer: task %q has no rubric criteria", task.ID)
	}

	// Build cache key: SHA256(output + JSON(rubric)) to prevent cross-contamination.
	rubricJSON, _ := json.Marshal(task.Rubric)
	cacheContent := output + string(rubricJSON)
	cacheKey := cache.CacheKey(cache.JudgeCacheKeyPrefix, cacheContent)

	// Cache lookup — avoids redundant gateway calls.
	if r.Cache != nil {
		if cached, err := r.Cache.Get(ctx, cacheKey); err == nil {
			var result ScoreResult
			if jsonErr := json.Unmarshal(cached, &result); jsonErr == nil {
				return result, nil
			}
		}
	}

	// Build judge prompt.
	systemPrompt := buildRubricSystemPrompt(task.Rubric.Criteria)
	userPrompt := fmt.Sprintf("Model output to evaluate:\n\n%s", output)

	model := r.Model
	if model == "" {
		model = "claude-3-5-haiku-20241022"
	}

	reqBody, err := json.Marshal(judgeRequest{
		Model: model,
		Messages: []judgeRequestMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		ResponseFormat: &judgeResponseFormat{Type: "json_object"},
		MaxTokens:      1024,
	})
	if err != nil {
		return ScoreResult{}, fmt.Errorf("rubric scorer marshal request: %w", err)
	}

	client := r.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: r.Timeout}
	}

	reqCtx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	url := r.GatewayURL + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return ScoreResult{}, fmt.Errorf("%w: building gateway request: %v", ErrPreconditionNotMet, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return ScoreResult{}, fmt.Errorf("%w: gateway unavailable: %v", ErrPreconditionNotMet, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ScoreResult{}, fmt.Errorf("%w: gateway HTTP %d", ErrPreconditionNotMet, resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ScoreResult{}, fmt.Errorf("rubric scorer read response: %w", err)
	}

	// Parse OpenAI-compatible chat response to extract content.
	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil || len(chatResp.Choices) == 0 {
		return ScoreResult{}, fmt.Errorf("rubric scorer parse chat response: %w", err)
	}

	var judgeResp judgeResponse
	if err := json.Unmarshal([]byte(chatResp.Choices[0].Message.Content), &judgeResp); err != nil {
		return ScoreResult{}, fmt.Errorf("rubric scorer parse judge JSON: %w", err)
	}

	score, rationale := computeWeightedMean(task.Rubric.Criteria, judgeResp.CriterionScores)

	result := ScoreResult{
		Score:     score,
		Metrics:   buildCriterionMetrics(judgeResp.CriterionScores),
		Rationale: rationale,
		Passed:    score >= task.Threshold,
	}

	// Cache the result; skip if zero score (may indicate transient judge error).
	if r.Cache != nil && score > 0.0 {
		if encoded, err := json.Marshal(result); err == nil {
			judgeHash := sha256.Sum256([]byte(cacheContent))
			_ = judgeHash // already embedded in cacheKey
			_ = r.Cache.Set(ctx, cacheKey, encoded, time.Hour)
		}
	}

	return result, nil
}

// computeWeightedMean computes the normalized weighted mean from criterion scores.
// Weights need not sum to 1.0 — they are normalized internally.
func computeWeightedMean(criteria []schema.RubricCriteria, scores []criterionScore) (float64, string) {
	// Build lookup map from judge response.
	scoreMap := make(map[string]criterionScore, len(scores))
	for _, s := range scores {
		scoreMap[s.Name] = s
	}

	var weightedSum, totalWeight float64
	var rationales []string

	for _, c := range criteria {
		if s, ok := scoreMap[c.Name]; ok {
			weightedSum += s.Score * c.Weight
			totalWeight += c.Weight
			if s.Rationale != "" {
				rationales = append(rationales, fmt.Sprintf("[%s] %s", c.Name, s.Rationale))
			}
		}
	}

	if totalWeight == 0 {
		return 0.0, ""
	}

	combined := ""
	for i, r := range rationales {
		if i > 0 {
			combined += " | "
		}
		combined += r
	}

	return weightedSum / totalWeight, combined
}

// buildCriterionMetrics converts judge criterion scores to a metrics map.
func buildCriterionMetrics(scores []criterionScore) map[string]float64 {
	m := make(map[string]float64, len(scores))
	for _, s := range scores {
		m["criterion_"+s.Name] = s.Score
	}
	return m
}

// buildRubricSystemPrompt builds the judge system prompt from rubric criteria.
func buildRubricSystemPrompt(criteria []schema.RubricCriteria) string {
	var buf bytes.Buffer
	buf.WriteString("You are an expert evaluator. Score the provided model output against each criterion below.\n\n")
	buf.WriteString("Scoring scale: 0.0 = completely fails criterion, 1.0 = fully meets criterion.\n\n")
	buf.WriteString("Criteria:\n")
	for _, c := range criteria {
		fmt.Fprintf(&buf, "- %s (weight %.2f): %s\n", c.Name, c.Weight, c.Description)
	}
	buf.WriteString("\nRespond with valid JSON matching this exact schema:\n")
	buf.WriteString(`{"criterion_scores": [{"name": "...", "score": 0.0, "rationale": "..."}], "overall": 0.0}`)
	buf.WriteString("\n\nEvaluate each criterion independently. Be precise and consistent.")
	return buf.String()
}

// ensure errors import is used.
var _ = errors.New
