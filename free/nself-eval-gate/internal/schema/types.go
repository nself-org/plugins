// Package schema provides shared type definitions and validation utilities
// for the nself-eval-gate eval harness.
package schema

import "time"

// EvalSuite represents a versioned collection of eval tasks registered in np_eval_suites.
// Purpose: Ground-truth typed representation of a suite row; shared across DB layer and scorer.
// Inputs: deserialized from DB via pgx or from YAML via validator.
// Outputs: passed to AggregateScorer.Run and HTTP handlers.
// Constraints: Slug must be unique per source_account_id; SchemaVer must be "1" for P4 suites.
type EvalSuite struct {
	ID              string    `json:"id" db:"id"`
	Slug            string    `json:"slug" db:"slug"`
	Version         string    `json:"version" db:"version"`
	SuiteType       string    `json:"suite_type" db:"suite_type"` // recall | generation | mixed
	Description     string    `json:"description,omitempty" db:"description"`
	Repo            string    `json:"repo" db:"repo"`
	SchemaVer       string    `json:"schema_ver" db:"schema_ver"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
	SourceAccountID string    `json:"source_account_id" db:"source_account_id"`
}

// EvalTask represents a single task within an eval suite, stored in np_eval_tasks.
// Purpose: Typed container for task spec including scoring mode, golden data, and thresholds.
// Inputs: loaded from DB or parsed from YAML eval-set file.
// Outputs: passed to individual scorers (ExactScorer, SemanticScorer, RubricScorer).
// Constraints: ScoringMode must be one of exact|semantic|rubric; ExpectedOutput required for
// exact/semantic; Rubric required for rubric mode. Threshold in [0,1].
type EvalTask struct {
	ID                     string         `json:"id" db:"task_ref" yaml:"id"`
	SuiteID                string         `json:"suite_id,omitempty" db:"suite_id"`
	Query                  string         `json:"query" db:"query" yaml:"query"`
	ScoringMode            string         `json:"scoring_mode" db:"scoring_mode" yaml:"scoring_mode"`
	ExpectedOutput         string         `json:"expected_output,omitempty" db:"expected_output" yaml:"expected_output,omitempty"`
	Rubric                 *Rubric        `json:"rubric,omitempty" db:"rubric" yaml:"rubric,omitempty"`
	GoldenMemories         []GoldenMemory `json:"golden_memories,omitempty" db:"golden_memories" yaml:"golden_memories,omitempty"`
	ExpectedOutputContains []string       `json:"expected_output_contains,omitempty" db:"expected_output_contains" yaml:"expected_output_contains,omitempty"`
	Metrics                []string       `json:"metrics" db:"metrics" yaml:"metrics"`
	Threshold              float64        `json:"threshold" db:"threshold" yaml:"threshold"`
	Tier                   string         `json:"tier,omitempty" db:"tier" yaml:"tier,omitempty"`
	Partial                bool           `json:"partial,omitempty" yaml:"partial,omitempty"`
	CreatedAt              time.Time      `json:"created_at,omitempty" db:"created_at"`
	SourceAccountID        string         `json:"source_account_id,omitempty" db:"source_account_id"`
}

// Rubric defines the LLM-as-judge scoring criteria for rubric mode tasks.
// Purpose: Encapsulates criteria list for structured judge prompt construction.
// Inputs: parsed from YAML rubric field or DB rubric JSONB column.
// Outputs: passed to RubricScorer to build judge prompt.
// Constraints: Criteria slice must be non-empty for rubric scoring mode.
type Rubric struct {
	Criteria []RubricCriteria `json:"criteria" yaml:"criteria"`
}

// RubricCriteria represents a single scoring dimension for LLM-as-judge evaluation.
// Purpose: Named, weighted dimension contributing to weighted-mean rubric score.
// Inputs: from YAML criteria array or DB rubric JSONB.
// Outputs: included in judge prompt; criterion name matched in judge response.
// Constraints: Weight is unnormalized (AggregateScorer normalizes); must be > 0.
type RubricCriteria struct {
	Name        string  `json:"name" yaml:"name"`
	Weight      float64 `json:"weight" yaml:"weight"`
	Description string  `json:"description" yaml:"description"`
}

// GoldenMemory represents a subject-predicate-object triple for recall evaluation.
// Purpose: Ground-truth fact triple used in precision@k / recall@k / fact_f1 computation.
// Inputs: from YAML golden_memories field or np_eval_tasks.golden_memories JSONB.
// Outputs: compared against retrieved triples in RecallQualityEval.
// Constraints: All three fields required; empty strings are invalid.
type GoldenMemory struct {
	Subject   string `json:"subject" yaml:"subject"`
	Predicate string `json:"predicate" yaml:"predicate"`
	Object    string `json:"object" yaml:"object"`
}

// EvalRun represents a completed eval suite execution stored in np_eval_runs.
// Purpose: Persisted record of a single suite run with per-task breakdown and aggregate metrics.
// Inputs: written by AggregateScorer after running all tasks.
// Outputs: read by gate.go for tier-clearance checks; returned by GET /eval/runs/{id}.
// Constraints: PassRate and SuiteScore are [0,1]; Passed = SuiteScore >= suite threshold.
type EvalRun struct {
	ID              string        `json:"id" db:"id"`
	SuiteID         string        `json:"suite_id" db:"suite_id"`
	TriggeredBy     string        `json:"triggered_by" db:"triggered_by"` // ci | manual | pre-tier-promotion
	CommitSHA       string        `json:"commit_sha,omitempty" db:"commit_sha"`
	Branch          string        `json:"branch,omitempty" db:"branch"`
	PassRate        float64       `json:"pass_rate" db:"pass_rate"`
	SuiteScore      float64       `json:"suite_score" db:"suite_score"`
	Passed          bool          `json:"passed" db:"passed"`
	Results         []TaskResult  `json:"tasks" db:"results"`
	DurationMS      int           `json:"duration_ms,omitempty" db:"duration_ms"`
	ModelJudge      string        `json:"model_judge,omitempty" db:"model_judge"`
	CreatedAt       time.Time     `json:"created_at" db:"created_at"`
	SourceAccountID string        `json:"source_account_id" db:"source_account_id"`
}

// TaskResult holds the per-task scoring output within an EvalRun.
// Purpose: Per-task breakdown stored in np_eval_runs.results JSONB column.
// Inputs: produced by individual scorers (ExactScorer, SemanticScorer, RubricScorer).
// Outputs: embedded in EvalRun.Results; surfaced in CLI table output and /eval/runs/{id}.
// Constraints: Score in [0,1]; Passed = Score >= task.Threshold.
type TaskResult struct {
	ID        string             `json:"id"`
	Score     float64            `json:"score"`
	Metrics   map[string]float64 `json:"metrics"`
	Passed    bool               `json:"passed"`
	Rationale string             `json:"rationale,omitempty"`
}

// EvalSuiteFile is the top-level YAML eval-set file structure (maps to eval-set-v1.json).
// Purpose: Deserialization target for YAML eval files loaded from {repo}/.claude/evals/.
// Inputs: YAML bytes read from disk; validated via ValidateEvalSet before use.
// Outputs: converted to []EvalTask for suite registration and scorer calls.
// Constraints: Version must be "1"; Suite slug must be unique; Tasks non-empty.
type EvalSuiteFile struct {
	Version     string     `yaml:"version" json:"version"`
	Suite       string     `yaml:"suite" json:"suite"`
	Description string     `yaml:"description,omitempty" json:"description,omitempty"`
	Repo        string     `yaml:"repo" json:"repo"`
	Tasks       []EvalTask `yaml:"tasks" json:"tasks"`
}

// ValidationError carries a field path and human-readable message from YAML schema validation.
// Purpose: Typed error returned by ValidateEvalSet for each schema violation.
// Inputs: produced by gojsonschema validation results.
// Outputs: returned by /eval/validate endpoint and CLI --validate-only flag.
// Constraints: Field uses dot-notation (e.g. "tasks[0].threshold").
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}
