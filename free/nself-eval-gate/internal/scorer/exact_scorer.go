package scorer

import (
	"context"
	"unicode"

	"github.com/nself-org/nself-eval-gate/internal/schema"
	"golang.org/x/text/unicode/norm"
)

// ExactScorer compares model output against expected output using string equality.
// Purpose: Fast, deterministic scorer for tasks with a single correct answer.
// Inputs: output (model response), task.ExpectedOutput (ground truth),
//
//	task.Partial=true enables Levenshtein similarity instead of exact match.
//
// Outputs: Score=1.0 on exact match, 0.0 on mismatch; Levenshtein [0,1] when Partial=true.
// Constraints: Normalization applied before comparison (Unicode lowercase, trim, whitespace collapse).
type ExactScorer struct{}

// Score computes exact-match or Levenshtein similarity between output and expected.
func (e *ExactScorer) Score(_ context.Context, output string, task schema.EvalTask) (ScoreResult, error) {
	normalized := normalizeText(output)
	expected := normalizeText(task.ExpectedOutput)

	var score float64
	if task.Partial {
		score = levenshteinSimilarity(normalized, expected)
	} else {
		if normalized == expected {
			score = 1.0
		} else {
			score = 0.0
		}
	}

	return ScoreResult{
		Score:   score,
		Metrics: map[string]float64{"exact_match": score},
		Passed:  score >= task.Threshold,
	}, nil
}

// normalizeText applies Unicode NFC normalization, lowercases, trims whitespace,
// and collapses internal runs of whitespace to a single space.
func normalizeText(s string) string {
	// NFC normalization (canonical decomposition + canonical composition).
	s = norm.NFC.String(s)

	// Unicode-aware lowercase.
	runes := []rune(s)
	for i, r := range runes {
		runes[i] = unicode.ToLower(r)
	}
	s = string(runes)

	// Trim leading/trailing whitespace.
	s = trimSpace(s)

	// Collapse internal whitespace runs.
	s = collapseWhitespace(s)

	return s
}

// trimSpace removes leading and trailing Unicode whitespace.
func trimSpace(s string) string {
	runes := []rune(s)
	start := 0
	for start < len(runes) && unicode.IsSpace(runes[start]) {
		start++
	}
	end := len(runes)
	for end > start && unicode.IsSpace(runes[end-1]) {
		end--
	}
	return string(runes[start:end])
}

// collapseWhitespace replaces any run of whitespace characters with a single space.
func collapseWhitespace(s string) string {
	runes := []rune(s)
	result := make([]rune, 0, len(runes))
	inSpace := false
	for _, r := range runes {
		if unicode.IsSpace(r) {
			if !inSpace {
				result = append(result, ' ')
				inSpace = true
			}
		} else {
			result = append(result, r)
			inSpace = false
		}
	}
	return string(result)
}

// levenshteinSimilarity computes 1 - (editDistance / maxLen) as a [0,1] similarity score.
// Returns 1.0 when both strings are empty (identical).
func levenshteinSimilarity(a, b string) float64 {
	ra := []rune(a)
	rb := []rune(b)
	maxLen := len(ra)
	if len(rb) > maxLen {
		maxLen = len(rb)
	}
	if maxLen == 0 {
		return 1.0
	}
	dist := levenshteinDistance(ra, rb)
	return 1.0 - float64(dist)/float64(maxLen)
}

// levenshteinDistance computes the edit distance between two rune slices.
func levenshteinDistance(a, b []rune) int {
	la, lb := len(a), len(b)
	dp := make([][]int, la+1)
	for i := range dp {
		dp[i] = make([]int, lb+1)
		dp[i][0] = i
	}
	for j := 0; j <= lb; j++ {
		dp[0][j] = j
	}
	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1]
			} else {
				m := dp[i-1][j] + 1 // deletion
				if dp[i][j-1]+1 < m {
					m = dp[i][j-1] + 1 // insertion
				}
				if dp[i-1][j-1]+1 < m {
					m = dp[i-1][j-1] + 1 // substitution
				}
				dp[i][j] = m
			}
		}
	}
	return dp[la][lb]
}
