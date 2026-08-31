package schema

import (
	"testing"
)

const validSuiteYAML = `
version: "1"
suite: "test-suite"
repo: "nclaw"
tasks:
  - id: "t001"
    query: "What medications does the user take?"
    scoring_mode: "rubric"
    rubric:
      criteria:
        - name: "accuracy"
          weight: 0.6
          description: "All medications match golden set"
        - name: "completeness"
          weight: 0.4
          description: "No medications omitted"
    metrics: ["precision_at_3", "fact_f1"]
    threshold: 0.80
    tier: "semi-auto"
`

const validExactSuiteYAML = `
version: "1"
suite: "exact-test"
repo: "clawde"
tasks:
  - id: "t001"
    query: "What is 2+2?"
    scoring_mode: "exact"
    expected_output: "4"
    metrics: ["exact_match"]
    threshold: 1.0
`

func TestValidatorValidSuite(t *testing.T) {
	errs, err := ValidateEvalSet([]byte(validSuiteYAML))
	if err != nil {
		t.Fatalf("ValidateEvalSet error: %v", err)
	}
	if len(errs) != 0 {
		for _, e := range errs {
			t.Errorf("unexpected validation error: field=%q msg=%q", e.Field, e.Message)
		}
	}
}

func TestValidatorValidExactSuite(t *testing.T) {
	errs, err := ValidateEvalSet([]byte(validExactSuiteYAML))
	if err != nil {
		t.Fatalf("ValidateEvalSet error: %v", err)
	}
	if len(errs) != 0 {
		for _, e := range errs {
			t.Errorf("unexpected validation error: field=%q msg=%q", e.Field, e.Message)
		}
	}
}

func TestValidatorMissingThreshold(t *testing.T) {
	yaml := `
version: "1"
suite: "missing-threshold"
repo: "nclaw"
tasks:
  - id: "t001"
    query: "test"
    scoring_mode: "exact"
    expected_output: "answer"
    metrics: ["exact_match"]
`
	errs, err := ValidateEvalSet([]byte(yaml))
	if err != nil {
		t.Fatalf("ValidateEvalSet error: %v", err)
	}
	if len(errs) == 0 {
		t.Error("expected validation errors for missing threshold, got none")
	}
	found := false
	for _, e := range errs {
		if e.Field == "threshold" || containsString(e.Field, "threshold") || containsString(e.Message, "threshold") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error mentioning 'threshold', got: %+v", errs)
	}
}

func TestValidatorInvalidScoringMode(t *testing.T) {
	yaml := `
version: "1"
suite: "bad-mode"
repo: "nclaw"
tasks:
  - id: "t001"
    query: "test"
    scoring_mode: "fuzzy"
    expected_output: "answer"
    metrics: ["exact_match"]
    threshold: 0.8
`
	errs, err := ValidateEvalSet([]byte(yaml))
	if err != nil {
		t.Fatalf("ValidateEvalSet error: %v", err)
	}
	if len(errs) == 0 {
		t.Error("expected validation errors for invalid scoring_mode, got none")
	}
}

func TestValidatorMissingRequiredFields(t *testing.T) {
	yaml := `
version: "1"
tasks:
  - id: "t001"
    query: "test"
    scoring_mode: "exact"
    metrics: ["exact_match"]
    threshold: 0.8
`
	errs, err := ValidateEvalSet([]byte(yaml))
	if err != nil {
		t.Fatalf("ValidateEvalSet error: %v", err)
	}
	// Should fail — missing required "suite" and "repo" fields.
	if len(errs) == 0 {
		t.Error("expected validation errors for missing suite and repo, got none")
	}
}

func TestValidatorRubricModeWithoutCriteria(t *testing.T) {
	yaml := `
version: "1"
suite: "no-criteria"
repo: "nclaw"
tasks:
  - id: "t001"
    query: "test"
    scoring_mode: "rubric"
    metrics: ["fact_f1"]
    threshold: 0.8
`
	errs, err := ValidateEvalSet([]byte(yaml))
	if err != nil {
		t.Fatalf("ValidateEvalSet error: %v", err)
	}
	// Rubric mode without rubric field should fail schema validation.
	if len(errs) == 0 {
		t.Error("expected validation errors for rubric mode without rubric criteria, got none")
	}
}

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}())
}
