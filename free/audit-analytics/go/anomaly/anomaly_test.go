// Purpose: Baseline tests for the anomaly detection package.
// Inputs: z-score float64 values, anomaly type constants, PrivilegedActions map.
// Outputs: severity strings, bool lookups, constant values.
// Constraints: No DB connection required; all tested functions are pure.
package anomaly

import "testing"

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
