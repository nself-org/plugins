package scorer

import (
	"context"
	"testing"

	"github.com/nself-org/nself-eval-gate/internal/schema"
)

func TestExactScorerNormalization(t *testing.T) {
	scorer := &ExactScorer{}
	ctx := context.Background()

	tests := []struct {
		name     string
		output   string
		expected string
		wantPass bool
		partial  bool
	}{
		{
			name: "identical strings",
			output: "hello world", expected: "hello world",
			wantPass: true,
		},
		{
			name: "case normalization",
			output: "Hello World", expected: "hello world",
			wantPass: true,
		},
		{
			name: "whitespace collapse",
			output: "hello   world", expected: "hello world",
			wantPass: true,
		},
		{
			name: "tab normalization",
			output: "hello\tworld", expected: "hello world",
			wantPass: true,
		},
		{
			name: "leading/trailing trim",
			output: "  hello world  ", expected: "hello world",
			wantPass: true,
		},
		{
			name: "unicode normalization — accented",
			output: "café", expected: "café",
			wantPass: true,
		},
		{
			name: "mismatch",
			output: "hello earth", expected: "hello world",
			wantPass: false,
		},
		{
			name: "empty both",
			output: "", expected: "",
			wantPass: true, // 1.0 >= default 0 threshold
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := schema.EvalTask{
				ID:             "t-test",
				Query:          "test query",
				ScoringMode:    "exact",
				ExpectedOutput: tt.expected,
				Metrics:        []string{"exact_match"},
				Threshold:      0.5,
				Partial:        tt.partial,
			}
			result, err := scorer.Score(ctx, tt.output, task)
			if err != nil {
				t.Fatalf("Score error: %v", err)
			}
			if result.Passed != tt.wantPass {
				t.Errorf("Passed=%v, want %v (score=%.3f)", result.Passed, tt.wantPass, result.Score)
			}
		})
	}
}

func TestExactScorerPartialLevenshtein(t *testing.T) {
	scorer := &ExactScorer{}
	ctx := context.Background()

	task := schema.EvalTask{
		ID:             "t-partial",
		Query:          "test",
		ScoringMode:    "exact",
		ExpectedOutput: "hello world",
		Metrics:        []string{"exact_match"},
		Threshold:      0.7,
		Partial:        true,
	}

	// "hello world" vs "hello world" — exact match in partial mode = 1.0
	result, err := scorer.Score(ctx, "hello world", task)
	if err != nil {
		t.Fatalf("Score error: %v", err)
	}
	if result.Score < 0.999 {
		t.Errorf("expected score ~1.0 for identical strings, got %.3f", result.Score)
	}

	// "hello earth" vs "hello world" — close but not identical
	result2, _ := scorer.Score(ctx, "hello earth", task)
	if result2.Score <= 0 || result2.Score >= 1 {
		t.Errorf("expected partial score between 0 and 1, got %.3f", result2.Score)
	}

	// completely different string — score should be low
	result3, _ := scorer.Score(ctx, "completely different text response here", task)
	if result3.Score >= 0.9 {
		t.Errorf("expected low score for very different strings, got %.3f", result3.Score)
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"kitten", "sitting", 3},
		{"abc", "", 3},
		{"", "abc", 3},
	}
	for _, tt := range tests {
		got := levenshteinDistance([]rune(tt.a), []rune(tt.b))
		if got != tt.want {
			t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
