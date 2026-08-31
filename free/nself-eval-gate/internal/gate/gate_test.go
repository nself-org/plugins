package gate

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/nself-eval-gate/internal/schema"
)

// mockStore implements db.Store for gate tests.
type mockStore struct {
	thresholds map[string]*schema.EvalThreshold
	suites     map[string]*schema.EvalSuite
	latestRuns map[string]*schema.EvalRun
}

func (m *mockStore) GetThresholdByTier(_ context.Context, tier string) (*schema.EvalThreshold, error) {
	t, ok := m.thresholds[tier]
	if !ok {
		return nil, nil
	}
	return t, nil
}

func (m *mockStore) GetSuiteBySlug(_ context.Context, slug, _ string) (*schema.EvalSuite, error) {
	s, ok := m.suites[slug]
	if !ok {
		return nil, nil
	}
	return s, nil
}

func (m *mockStore) GetLatestRunBySuite(_ context.Context, suiteID, _ string) (*schema.EvalRun, error) {
	run, ok := m.latestRuns[suiteID]
	if !ok {
		return nil, nil
	}
	return run, nil
}

// Implement remaining Store interface methods as no-ops.
func (m *mockStore) InsertSuite(_ context.Context, _ *schema.EvalSuite) error { return nil }
func (m *mockStore) GetSuite(_ context.Context, _, _ string) (*schema.EvalSuite, error) {
	return nil, nil
}
func (m *mockStore) ListSuites(_ context.Context, _ string) ([]schema.EvalSuite, error) {
	return nil, nil
}
func (m *mockStore) InsertTask(_ context.Context, _ *schema.EvalTask) error { return nil }
func (m *mockStore) ListTasksBySuite(_ context.Context, _, _ string) ([]schema.EvalTask, error) {
	return nil, nil
}
func (m *mockStore) InsertRun(_ context.Context, _ *schema.EvalRun) error { return nil }
func (m *mockStore) GetRun(_ context.Context, _, _ string) (*schema.EvalRun, error) {
	return nil, nil
}
func (m *mockStore) ListThresholds(_ context.Context) ([]schema.EvalThreshold, error) {
	return nil, nil
}

func TestSupervisedTierAlwaysClears(t *testing.T) {
	store := &mockStore{
		thresholds: map[string]*schema.EvalThreshold{
			"supervised": {
				AutononyTier:  "supervised",
				MinPassRate:   0.0,
				MinSuiteScore: 0.0,
				AppliesTo:     []string{},
				Enforced:      false,
				UpdatedAt:     time.Now(),
			},
		},
	}

	result, err := IsTierCleared(context.Background(), "supervised", store, "primary")
	if err != nil {
		t.Fatalf("IsTierCleared error: %v", err)
	}
	if !result.Cleared {
		t.Errorf("supervised tier must always clear; got Cleared=false")
	}
	if len(result.BlockingSuites) != 0 {
		t.Errorf("supervised tier should have no blocking suites, got: %v", result.BlockingSuites)
	}
}

func TestTierClearedWhenAllSuitesPass(t *testing.T) {
	store := &mockStore{
		thresholds: map[string]*schema.EvalThreshold{
			"semi-auto": {
				AutononyTier:  "semi-auto",
				MinPassRate:   0.80,
				MinSuiteScore: 0.75,
				AppliesTo:     []string{"recall-quality-v1"},
				Enforced:      true,
			},
		},
		suites: map[string]*schema.EvalSuite{
			"recall-quality-v1": {ID: "suite-uuid-1", Slug: "recall-quality-v1"},
		},
		latestRuns: map[string]*schema.EvalRun{
			"suite-uuid-1": {
				PassRate:   0.85,
				SuiteScore: 0.80,
				Passed:     true,
				CreatedAt:  time.Now(),
			},
		},
	}

	result, err := IsTierCleared(context.Background(), "semi-auto", store, "primary")
	if err != nil {
		t.Fatalf("IsTierCleared error: %v", err)
	}
	if !result.Cleared {
		t.Errorf("expected Cleared=true when pass_rate=0.85 >= 0.80 and suite_score=0.80 >= 0.75")
	}
}

func TestTierBlockedWhenSuiteScoreBelowThreshold(t *testing.T) {
	store := &mockStore{
		thresholds: map[string]*schema.EvalThreshold{
			"semi-auto": {
				AutononyTier:  "semi-auto",
				MinPassRate:   0.80,
				MinSuiteScore: 0.75,
				AppliesTo:     []string{"recall-quality-v1"},
				Enforced:      true,
			},
		},
		suites: map[string]*schema.EvalSuite{
			"recall-quality-v1": {ID: "suite-uuid-1", Slug: "recall-quality-v1"},
		},
		latestRuns: map[string]*schema.EvalRun{
			"suite-uuid-1": {
				PassRate:   0.82,
				SuiteScore: 0.70, // below 0.75 threshold
				Passed:     false,
			},
		},
	}

	result, err := IsTierCleared(context.Background(), "semi-auto", store, "primary")
	if err != nil {
		t.Fatalf("IsTierCleared error: %v", err)
	}
	if result.Cleared {
		t.Error("expected Cleared=false when suite_score below threshold")
	}
	if len(result.BlockingSuites) == 0 {
		t.Error("expected blocking suites list, got empty")
	}
}

func TestTierBlockedWhenNoRunsExist(t *testing.T) {
	store := &mockStore{
		thresholds: map[string]*schema.EvalThreshold{
			"full-auto": {
				AutononyTier:  "full-auto",
				MinPassRate:   0.92,
				MinSuiteScore: 0.88,
				AppliesTo:     []string{"recall-quality-v1", "generation-v1"},
				Enforced:      true,
			},
		},
		suites: map[string]*schema.EvalSuite{
			"recall-quality-v1": {ID: "suite-uuid-1", Slug: "recall-quality-v1"},
			"generation-v1":     {ID: "suite-uuid-2", Slug: "generation-v1"},
		},
		latestRuns: map[string]*schema.EvalRun{
			// No runs for suite-uuid-1 or suite-uuid-2
		},
	}

	result, err := IsTierCleared(context.Background(), "full-auto", store, "primary")
	if err != nil {
		t.Fatalf("IsTierCleared error: %v", err)
	}
	if result.Cleared {
		t.Error("expected Cleared=false when no runs exist")
	}
	if len(result.BlockingSuites) != 2 {
		t.Errorf("expected 2 blocking suites (no runs), got %d: %v", len(result.BlockingSuites), result.BlockingSuites)
	}
}
