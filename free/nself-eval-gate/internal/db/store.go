// Package db provides the database access layer for nself-eval-gate.
// All operations use pgx/v5 for type-safe Postgres access.
package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-eval-gate/internal/schema"
)

// Store defines all data-access operations required by the eval-gate HTTP handlers,
// aggregate scorer, and CI gate logic.
// Purpose: Typed interface over Postgres; decouples business logic from pgx details.
// Inputs: context.Context for all calls (honors cancellation and timeouts).
// Outputs: domain types from internal/schema; pgx errors wrapped with context.
// Constraints: All methods use source_account_id for multi-app isolation where applicable.
type Store interface {
	// Suite operations
	InsertSuite(ctx context.Context, suite *schema.EvalSuite) error
	GetSuite(ctx context.Context, id string, sourceAccountID string) (*schema.EvalSuite, error)
	GetSuiteBySlug(ctx context.Context, slug string, sourceAccountID string) (*schema.EvalSuite, error)
	ListSuites(ctx context.Context, sourceAccountID string) ([]schema.EvalSuite, error)

	// Task operations
	InsertTask(ctx context.Context, task *schema.EvalTask) error
	ListTasksBySuite(ctx context.Context, suiteID string, sourceAccountID string) ([]schema.EvalTask, error)

	// Run operations
	InsertRun(ctx context.Context, run *schema.EvalRun) error
	GetRun(ctx context.Context, id string, sourceAccountID string) (*schema.EvalRun, error)
	GetLatestRunBySuite(ctx context.Context, suiteID string, sourceAccountID string) (*schema.EvalRun, error)

	// Threshold operations (global — no source_account_id filtering)
	ListThresholds(ctx context.Context) ([]schema.EvalThreshold, error)
	GetThresholdByTier(ctx context.Context, tier string) (*schema.EvalThreshold, error)
}

// PostgresStore implements Store over a pgx connection pool.
// Purpose: Production implementation using pgx/v5 for type-safe Postgres access.
// Inputs: pgxpool.Pool initialized in main.go from NSELF_EVAL_GATE_DB_URL.
// Outputs: domain objects with all fields populated from Postgres rows.
// Constraints: Pool must remain open for the lifetime of the plugin process.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a PostgresStore wrapping the given pgx pool.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// EvalThreshold mirrors np_eval_thresholds for gate checks.
// Kept in db package to avoid import cycles with schema package.
type EvalThreshold = schema.EvalThreshold

// Compile-time interface check.
var _ Store = (*PostgresStore)(nil)

// Ping verifies database connectivity. Used by the /health endpoint.
func (s *PostgresStore) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return s.pool.Ping(ctx)
}
