// Purpose: Baseline tests for the anomaly detection package.
// Inputs: z-score float64 values, anomaly type constants, PrivilegedActions map.
// Outputs: severity strings, bool lookups, constant values.
// Constraints: No DB connection required; all tested functions are pure.
package anomaly

import (
	"context"
	"strings"
	"testing"
)

// TestBuild verifies the anomaly package compiles and exports the expected constants.
func TestBuild(t *testing.T) {
	t.Log("anomaly package compiled OK")
	// Verify constants are non-empty strings.
	for _, c := range []string{TypeBulkDelete, TypeNewIP, TypeImpossibleTravel, TypeAfterHoursAdmin, TypeFreqSpike} {
		if c == "" {
			t.Errorf("anomaly type constant is empty")
		}
	}
	for _, s := range []string{SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical} {
		if s == "" {
			t.Errorf("severity constant is empty")
		}
	}
}

// TestSeverityForZScore verifies z-score → severity bucket mapping.
func TestSeverityForZScore(t *testing.T) {
	cases := []struct {
		z    float64
		want string
	}{
		{1.0, SeverityLow},
		{2.9, SeverityLow},
		{3.0, SeverityMedium},
		{4.9, SeverityMedium},
		{5.0, SeverityHigh},
		{7.9, SeverityHigh},
		{8.0, SeverityCritical},
		{100.0, SeverityCritical},
	}
	for _, tc := range cases {
		got := severityForZScore(tc.z)
		if got != tc.want {
			t.Errorf("severityForZScore(%.1f) = %q, want %q", tc.z, got, tc.want)
		}
	}
}

// TestPrivilegedActions verifies the PrivilegedActions map contains expected entries.
func TestPrivilegedActions(t *testing.T) {
	expected := []string{"role_grant", "plugin_install", "schema_alter", "gdpr_delete"}
	for _, action := range expected {
		if !PrivilegedActions[action] {
			t.Errorf("PrivilegedActions[%q] should be true", action)
		}
	}
	if PrivilegedActions["login"] {
		t.Error("PrivilegedActions[\"login\"] should be false")
	}
}

// Events whose actor is not a uuid (system events: np_audit_log.user_id is NULL for them) have no per-user
// behaviour to score: every detector's query must leave them out, or each would read as a new-IP or
// bulk-delete anomaly of a user that does not exist.
func TestDetectorQueriesSkipEventsWithoutAnActor(t *testing.T) {
	var sqls []string
	q := func(_ context.Context, sql string, _ ...interface{}) ([]map[string]interface{}, error) {
		sqls = append(sqls, sql)
		return nil, nil
	}
	e := func(context.Context, string, ...interface{}) (int64, error) { return 0, nil }
	for _, hours := range [][2]int{{9, 17}, {20, 6}} { // both branches of the after-hours query
		sqls = nil
		s := NewScorer(q, e, ScorerConfig{ZScoreThreshold: 3, AfterHoursStart: hours[0], AfterHoursEnd: hours[1]})
		if _, err := s.ScoreRecent(context.Background()); err != nil {
			t.Fatal(err)
		}
		if len(sqls) != 4 {
			t.Fatalf("hours %v: %d queries, want 4", hours, len(sqls))
		}
		for _, sql := range sqls {
			// every SELECT over the log filters the actor, in each of its CTEs
			if n, m := strings.Count(sql, "FROM np_audit_log"), strings.Count(sql, "user_id IS NOT NULL"); m < n {
				t.Errorf("hours %v: %d reads of np_audit_log but %d actor filters in:\n%s", hours, n, m, sql)
			}
		}
	}
}
