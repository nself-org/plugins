//go:build integration

// Package db integration tests require a live Postgres instance with migrations applied.
// Run: NSELF_EVAL_GATE_DB_URL=postgres://... go test -tags integration ./internal/db/...
package db

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-eval-gate/internal/schema"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("NSELF_EVAL_GATE_DB_URL")
	if url == "" {
		t.Skip("NSELF_EVAL_GATE_DB_URL not set; skipping integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestInsertAndGetSuite(t *testing.T) {
	pool := testPool(t)
	store := NewPostgresStore(pool)
	ctx := context.Background()

	suite := &schema.EvalSuite{
		Slug:            "test-suite-" + time.Now().Format("20060102150405"),
		Version:         "1.0.0",
		SuiteType:       "generation",
		Description:     "integration test suite",
		Repo:            "nclaw",
		SchemaVer:       "1",
		SourceAccountID: "test",
	}

	if err := store.InsertSuite(ctx, suite); err != nil {
		t.Fatalf("InsertSuite: %v", err)
	}
	if suite.ID == "" {
		t.Fatal("expected suite.ID populated after insert")
	}

	got, err := store.GetSuite(ctx, suite.ID, "test")
	if err != nil {
		t.Fatalf("GetSuite: %v", err)
	}
	if got == nil {
		t.Fatal("expected suite, got nil")
	}
	if got.Slug != suite.Slug {
		t.Errorf("slug mismatch: got %q, want %q", got.Slug, suite.Slug)
	}
}

func TestGetThresholdByTier(t *testing.T) {
	pool := testPool(t)
	store := NewPostgresStore(pool)
	ctx := context.Background()

	for _, tier := range []string{"supervised", "semi-auto", "full-auto"} {
		threshold, err := store.GetThresholdByTier(ctx, tier)
		if err != nil {
			t.Fatalf("GetThresholdByTier(%s): %v", tier, err)
		}
		if threshold == nil {
			t.Fatalf("expected threshold for tier %s, got nil (run migration 005?)", tier)
		}
		if threshold.AutononyTier != tier {
			t.Errorf("tier mismatch: got %q, want %q", threshold.AutononyTier, tier)
		}
	}

	supervised, _ := store.GetThresholdByTier(ctx, "supervised")
	if supervised.Enforced {
		t.Error("supervised tier must have enforced=false")
	}
}

func TestConcurrentInsertRun(t *testing.T) {
	pool := testPool(t)
	store := NewPostgresStore(pool)
	ctx := context.Background()

	// Create a suite to attach runs to.
	suite := &schema.EvalSuite{
		Slug:            "concurrent-test-" + time.Now().Format("20060102150405"),
		Version:         "1.0.0",
		SuiteType:       "generation",
		Repo:            "nclaw",
		SchemaVer:       "1",
		SourceAccountID: "test",
	}
	if err := store.InsertSuite(ctx, suite); err != nil {
		t.Fatalf("InsertSuite: %v", err)
	}

	var wg sync.WaitGroup
	const n = 10
	ids := make([]string, n)
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			run := &schema.EvalRun{
				SuiteID:         suite.ID,
				TriggeredBy:     "manual",
				PassRate:        0.9,
				SuiteScore:      0.85,
				Passed:          true,
				Results:         []schema.TaskResult{},
				SourceAccountID: "test",
			}
			errs[idx] = store.InsertRun(ctx, run)
			ids[idx] = run.ID
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d InsertRun error: %v", i, err)
		}
	}

	// Verify no duplicate IDs.
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" {
			t.Error("expected non-empty run ID")
			continue
		}
		if seen[id] {
			t.Errorf("duplicate run ID: %s", id)
		}
		seen[id] = true
	}
}
