// Purpose: HeatmapRefresher.Refresh creates a missing heatmap view before refreshing it.
// Constraints: no database; the exec function is a recorder.
package patterns

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestRefreshEnsuresViewFirst checks the statements Refresh runs and their order, with and without an ensure error.
func TestRefreshEnsuresViewFirst(t *testing.T) {
	cases := []struct {
		name       string
		failEnsure bool
	}{
		{"ensure succeeds", false},
		{"ensure fails, refresh still runs", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			exec := func(_ context.Context, sql string, _ ...interface{}) error {
				got = append(got, sql)
				if tc.failEnsure && len(got) == 1 {
					return errors.New("boom")
				}
				return nil
			}
			NewHeatmapRefresher(exec, 0).Refresh(context.Background())
			if len(got) != 2 {
				t.Fatalf("statements = %d, want 2: %q", len(got), got)
			}
			if !strings.Contains(got[0], "CREATE MATERIALIZED VIEW public.np_audit_heatmap") ||
				!strings.Contains(got[0], "to_regclass('public.np_audit_heatmap') IS NULL") {
				t.Errorf("first statement is not the guarded view create: %q", got[0])
			}
			if !strings.HasPrefix(got[1], "REFRESH MATERIALIZED VIEW CONCURRENTLY np_audit_heatmap") {
				t.Errorf("second statement = %q, want the refresh", got[1])
			}
		})
	}
}
