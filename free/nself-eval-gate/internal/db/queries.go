package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/nself-eval-gate/internal/schema"
)

// InsertSuite persists a new eval suite row.
// Purpose: Register a new eval suite in np_eval_suites.
// Inputs: suite with all required fields; suite.ID should be pre-populated (gen_random_uuid at DB level accepted).
// Outputs: suite.ID populated from DB-generated UUID on return.
// Constraints: Slug must be unique per source_account_id; returns pgx error on conflict.
func (s *PostgresStore) InsertSuite(ctx context.Context, suite *schema.EvalSuite) error {
	q := `
		INSERT INTO np_eval_suites (slug, version, suite_type, description, repo, schema_ver, source_account_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`
	return s.pool.QueryRow(ctx, q,
		suite.Slug, suite.Version, suite.SuiteType, suite.Description,
		suite.Repo, suite.SchemaVer, suite.SourceAccountID,
	).Scan(&suite.ID, &suite.CreatedAt, &suite.UpdatedAt)
}

// GetSuite retrieves a suite by primary key.
// Purpose: Fetch a single suite for handler and gate operations.
// Inputs: id (UUID string), sourceAccountID for multi-app isolation.
// Outputs: *EvalSuite or nil if not found (pgx.ErrNoRows → nil, nil return).
// Constraints: Returns error on DB failure; not-found returns nil suite without error.
func (s *PostgresStore) GetSuite(ctx context.Context, id, sourceAccountID string) (*schema.EvalSuite, error) {
	q := `SELECT id, slug, version, suite_type, description, repo, schema_ver, created_at, updated_at, source_account_id
		  FROM np_eval_suites WHERE id=$1 AND source_account_id=$2`
	var suite schema.EvalSuite
	err := s.pool.QueryRow(ctx, q, id, sourceAccountID).Scan(
		&suite.ID, &suite.Slug, &suite.Version, &suite.SuiteType, &suite.Description,
		&suite.Repo, &suite.SchemaVer, &suite.CreatedAt, &suite.UpdatedAt, &suite.SourceAccountID,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("GetSuite: %w", err)
	}
	return &suite, nil
}

// GetSuiteBySlug retrieves a suite by its slug.
// Purpose: Look up suite during CI run where slug is known but ID is not.
// Inputs: slug string, sourceAccountID.
// Outputs: *EvalSuite or nil if not found.
// Constraints: Uses unique index on (slug, source_account_id).
func (s *PostgresStore) GetSuiteBySlug(ctx context.Context, slug, sourceAccountID string) (*schema.EvalSuite, error) {
	q := `SELECT id, slug, version, suite_type, description, repo, schema_ver, created_at, updated_at, source_account_id
		  FROM np_eval_suites WHERE slug=$1 AND source_account_id=$2`
	var suite schema.EvalSuite
	err := s.pool.QueryRow(ctx, q, slug, sourceAccountID).Scan(
		&suite.ID, &suite.Slug, &suite.Version, &suite.SuiteType, &suite.Description,
		&suite.Repo, &suite.SchemaVer, &suite.CreatedAt, &suite.UpdatedAt, &suite.SourceAccountID,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("GetSuiteBySlug: %w", err)
	}
	return &suite, nil
}

// ListSuites returns all suites for a source account.
// Purpose: Used by GET /eval/suites and --all flag in CLI.
// Inputs: sourceAccountID for multi-app isolation.
// Outputs: slice of EvalSuite ordered by created_at DESC.
// Constraints: Empty slice (not nil) returned when no suites exist.
func (s *PostgresStore) ListSuites(ctx context.Context, sourceAccountID string) ([]schema.EvalSuite, error) {
	q := `SELECT id, slug, version, suite_type, description, repo, schema_ver, created_at, updated_at, source_account_id
		  FROM np_eval_suites WHERE source_account_id=$1 ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, q, sourceAccountID)
	if err != nil {
		return nil, fmt.Errorf("ListSuites: %w", err)
	}
	defer rows.Close()
	var suites []schema.EvalSuite
	for rows.Next() {
		var suite schema.EvalSuite
		if err := rows.Scan(
			&suite.ID, &suite.Slug, &suite.Version, &suite.SuiteType, &suite.Description,
			&suite.Repo, &suite.SchemaVer, &suite.CreatedAt, &suite.UpdatedAt, &suite.SourceAccountID,
		); err != nil {
			return nil, fmt.Errorf("ListSuites scan: %w", err)
		}
		suites = append(suites, suite)
	}
	if suites == nil {
		suites = []schema.EvalSuite{}
	}
	return suites, rows.Err()
}

// InsertTask persists a new eval task row.
// Purpose: Store a task from YAML eval-set loading or API suite registration.
// Inputs: task with SuiteID set; JSON serialized for Rubric and GoldenMemories JSONB columns.
// Outputs: task.ID populated from DB-generated UUID.
// Constraints: SuiteID must reference a valid np_eval_suites row (FK enforced).
func (s *PostgresStore) InsertTask(ctx context.Context, task *schema.EvalTask) error {
	rubricJSON, err := json.Marshal(task.Rubric)
	if err != nil {
		return fmt.Errorf("InsertTask marshal rubric: %w", err)
	}
	goldenJSON, err := json.Marshal(task.GoldenMemories)
	if err != nil {
		return fmt.Errorf("InsertTask marshal golden: %w", err)
	}
	q := `
		INSERT INTO np_eval_tasks
			(suite_id, task_ref, query, scoring_mode, expected_output, rubric, golden_memories,
			 expected_output_contains, metrics, threshold, tier, source_account_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id, created_at`
	return s.pool.QueryRow(ctx, q,
		task.SuiteID, task.ID, task.Query, task.ScoringMode, task.ExpectedOutput,
		rubricJSON, goldenJSON, task.ExpectedOutputContains, task.Metrics,
		task.Threshold, nilIfEmpty(task.Tier), task.SourceAccountID,
	).Scan(&task.ID, &task.CreatedAt)
}

// ListTasksBySuite returns all tasks belonging to a suite.
// Purpose: Load task set for AggregateScorer before running eval.
// Inputs: suiteID UUID, sourceAccountID.
// Outputs: ordered slice of EvalTask.
// Constraints: JSONB columns (Rubric, GoldenMemories) unmarshaled on read.
func (s *PostgresStore) ListTasksBySuite(ctx context.Context, suiteID, sourceAccountID string) ([]schema.EvalTask, error) {
	q := `SELECT id, suite_id, task_ref, query, scoring_mode, expected_output, rubric, golden_memories,
			 expected_output_contains, metrics, threshold, tier, created_at, source_account_id
		  FROM np_eval_tasks WHERE suite_id=$1 AND source_account_id=$2 ORDER BY created_at ASC`
	rows, err := s.pool.Query(ctx, q, suiteID, sourceAccountID)
	if err != nil {
		return nil, fmt.Errorf("ListTasksBySuite: %w", err)
	}
	defer rows.Close()
	var tasks []schema.EvalTask
	for rows.Next() {
		var task schema.EvalTask
		var rubricJSON []byte
		var goldenJSON []byte
		if err := rows.Scan(
			&task.ID, &task.SuiteID, &task.ID, &task.Query, &task.ScoringMode,
			&task.ExpectedOutput, &rubricJSON, &goldenJSON,
			&task.ExpectedOutputContains, &task.Metrics, &task.Threshold,
			&task.Tier, &task.CreatedAt, &task.SourceAccountID,
		); err != nil {
			return nil, fmt.Errorf("ListTasksBySuite scan: %w", err)
		}
		if len(rubricJSON) > 0 && string(rubricJSON) != "null" {
			var rubric schema.Rubric
			if err := json.Unmarshal(rubricJSON, &rubric); err != nil {
				return nil, fmt.Errorf("ListTasksBySuite unmarshal rubric: %w", err)
			}
			task.Rubric = &rubric
		}
		if len(goldenJSON) > 0 && string(goldenJSON) != "null" {
			if err := json.Unmarshal(goldenJSON, &task.GoldenMemories); err != nil {
				return nil, fmt.Errorf("ListTasksBySuite unmarshal golden: %w", err)
			}
		}
		tasks = append(tasks, task)
	}
	if tasks == nil {
		tasks = []schema.EvalTask{}
	}
	return tasks, rows.Err()
}

// InsertRun writes a completed eval run to np_eval_runs.
// Purpose: Persist AggregateScorer output for gate checks and history.
// Inputs: run with PassRate, SuiteScore, Passed, Results, and metadata set.
// Outputs: run.ID and run.CreatedAt populated from DB.
// Constraints: Results JSONB serialized from []TaskResult; SuiteID must exist.
func (s *PostgresStore) InsertRun(ctx context.Context, run *schema.EvalRun) error {
	resultsJSON, err := json.Marshal(run.Results)
	if err != nil {
		return fmt.Errorf("InsertRun marshal results: %w", err)
	}
	q := `
		INSERT INTO np_eval_runs
			(suite_id, triggered_by, commit_sha, branch, pass_rate, suite_score, passed,
			 results, duration_ms, model_judge, source_account_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id, created_at`
	return s.pool.QueryRow(ctx, q,
		run.SuiteID, run.TriggeredBy, run.CommitSHA, run.Branch,
		run.PassRate, run.SuiteScore, run.Passed,
		resultsJSON, run.DurationMS, run.ModelJudge, run.SourceAccountID,
	).Scan(&run.ID, &run.CreatedAt)
}

// GetRun retrieves a run by primary key.
// Purpose: Fetch full run result for GET /eval/runs/{id}.
// Inputs: id UUID, sourceAccountID.
// Outputs: *EvalRun with Results deserialized, or nil if not found.
// Constraints: Results JSONB deserialized on read.
func (s *PostgresStore) GetRun(ctx context.Context, id, sourceAccountID string) (*schema.EvalRun, error) {
	q := `SELECT id, suite_id, triggered_by, commit_sha, branch, pass_rate, suite_score, passed,
			 results, duration_ms, model_judge, created_at, source_account_id
		  FROM np_eval_runs WHERE id=$1 AND source_account_id=$2`
	var run schema.EvalRun
	var resultsJSON []byte
	err := s.pool.QueryRow(ctx, q, id, sourceAccountID).Scan(
		&run.ID, &run.SuiteID, &run.TriggeredBy, &run.CommitSHA, &run.Branch,
		&run.PassRate, &run.SuiteScore, &run.Passed, &resultsJSON,
		&run.DurationMS, &run.ModelJudge, &run.CreatedAt, &run.SourceAccountID,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("GetRun: %w", err)
	}
	if err := json.Unmarshal(resultsJSON, &run.Results); err != nil {
		return nil, fmt.Errorf("GetRun unmarshal results: %w", err)
	}
	return &run, nil
}

// GetLatestRunBySuite returns the most recent run for a suite.
// Purpose: Used by gate.go to check current tier clearance status.
// Inputs: suiteID UUID, sourceAccountID.
// Outputs: *EvalRun most recent by created_at DESC, or nil if no runs exist.
// Constraints: Uses index on (suite_id, created_at DESC) for efficiency.
func (s *PostgresStore) GetLatestRunBySuite(ctx context.Context, suiteID, sourceAccountID string) (*schema.EvalRun, error) {
	q := `SELECT id, suite_id, triggered_by, commit_sha, branch, pass_rate, suite_score, passed,
			 results, duration_ms, model_judge, created_at, source_account_id
		  FROM np_eval_runs WHERE suite_id=$1 AND source_account_id=$2
		  ORDER BY created_at DESC LIMIT 1`
	var run schema.EvalRun
	var resultsJSON []byte
	err := s.pool.QueryRow(ctx, q, suiteID, sourceAccountID).Scan(
		&run.ID, &run.SuiteID, &run.TriggeredBy, &run.CommitSHA, &run.Branch,
		&run.PassRate, &run.SuiteScore, &run.Passed, &resultsJSON,
		&run.DurationMS, &run.ModelJudge, &run.CreatedAt, &run.SourceAccountID,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("GetLatestRunBySuite: %w", err)
	}
	if err := json.Unmarshal(resultsJSON, &run.Results); err != nil {
		return nil, fmt.Errorf("GetLatestRunBySuite unmarshal: %w", err)
	}
	return &run, nil
}

// ListThresholds returns all autonomy-tier threshold rows.
// Purpose: Used by GET /eval/thresholds and gate initialization.
// Inputs: none (global — no source_account_id).
// Outputs: all three tier rows from np_eval_thresholds.
// Constraints: np_eval_thresholds has NO source_account_id (intentional global config).
func (s *PostgresStore) ListThresholds(ctx context.Context) ([]schema.EvalThreshold, error) {
	q := `SELECT id, autonomy_tier, min_pass_rate, min_suite_score, applies_to, enforced, updated_at
		  FROM np_eval_thresholds ORDER BY autonomy_tier`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("ListThresholds: %w", err)
	}
	defer rows.Close()
	var thresholds []schema.EvalThreshold
	for rows.Next() {
		var t schema.EvalThreshold
		if err := rows.Scan(&t.ID, &t.AutononyTier, &t.MinPassRate, &t.MinSuiteScore,
			&t.AppliesTo, &t.Enforced, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("ListThresholds scan: %w", err)
		}
		thresholds = append(thresholds, t)
	}
	if thresholds == nil {
		thresholds = []schema.EvalThreshold{}
	}
	return thresholds, rows.Err()
}

// GetThresholdByTier returns the threshold config for a specific autonomy tier.
// Purpose: Direct lookup for gate check per tier.
// Inputs: tier string (supervised|semi-auto|full-auto).
// Outputs: *EvalThreshold or nil if tier not seeded.
// Constraints: Uses UNIQUE index on autonomy_tier.
func (s *PostgresStore) GetThresholdByTier(ctx context.Context, tier string) (*schema.EvalThreshold, error) {
	q := `SELECT id, autonomy_tier, min_pass_rate, min_suite_score, applies_to, enforced, updated_at
		  FROM np_eval_thresholds WHERE autonomy_tier=$1`
	var t schema.EvalThreshold
	err := s.pool.QueryRow(ctx, q, tier).Scan(
		&t.ID, &t.AutononyTier, &t.MinPassRate, &t.MinSuiteScore, &t.AppliesTo, &t.Enforced, &t.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("GetThresholdByTier: %w", err)
	}
	return &t, nil
}

// nilIfEmpty returns nil interface{} for empty strings to store SQL NULL.
func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// ensure time import is used.
var _ = time.Second
