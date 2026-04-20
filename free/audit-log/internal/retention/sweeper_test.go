package retention

import (
	"os"
	"testing"
)

// TestSweeperDefault verifies that Sweeper respects the AUDIT_LOG_RETENTION_DAYS
// env var for non-tiered installations and defaults to 90 days when unset.
func TestSweeperDefault(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		want    int
	}{
		{"default (unset)", "", defaultRetentionDays},
		{"custom 180d", "180", 180},
		{"custom 365d", "365", 365},
		{"invalid (use default)", "invalid", defaultRetentionDays},
		{"zero (use default)", "0", defaultRetentionDays},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envVal != "" {
				t.Setenv("AUDIT_LOG_RETENTION_DAYS", tc.envVal)
			} else {
				os.Unsetenv("AUDIT_LOG_RETENTION_DAYS")
			}

			s := &Sweeper{retentionDays: retentionDaysForTier(TierDefault)}
			if s.retentionDays != tc.want {
				t.Errorf("retentionDays = %d, want %d", s.retentionDays, tc.want)
			}
		})
	}
}

// TestSweeperTierOverride verifies that Enterprise and Business tier overrides
// take precedence over the env var.
func TestSweeperTierOverride(t *testing.T) {
	// Set an env var that would otherwise affect the default tier.
	t.Setenv("AUDIT_LOG_RETENTION_DAYS", "30")

	tests := []struct {
		tier Tier
		want int
	}{
		{TierDefault, 30},              // env var respected
		{TierBusiness, businessRetentionDays},
		{TierEnterprise, enterpriseRetentionDays},
	}

	for _, tc := range tests {
		t.Run(string(tc.tier), func(t *testing.T) {
			got := retentionDaysForTier(tc.tier)
			if got != tc.want {
				t.Errorf("tier %s: retentionDays = %d, want %d", tc.tier, got, tc.want)
			}
		})
	}
}

// TestSweeperGDPR verifies that partitionMonth correctly parses the partition
// naming convention (np_auditlog_events_YYYY_MM) used in GDPR anonymization.
// This is the unit-level proof that partition-based sweeping correctly targets
// the right calendar months.
func TestSweeperGDPR(t *testing.T) {
	tests := []struct {
		partitionName string
		expectErr     bool
		expectedYear  int
		expectedMonth int
	}{
		{"np_auditlog_events_2024_01", false, 2024, 1},
		{"np_auditlog_events_2023_12", false, 2023, 12},
		{"np_auditlog_events_2026_03", false, 2026, 3},
		{"np_auditlog_events_invalid", true, 0, 0},
		{"np_auditlog_events_2024_13", true, 0, 0}, // month 13 is invalid
	}

	for _, tc := range tests {
		t.Run(tc.partitionName, func(t *testing.T) {
			parsed, parseErr := partitionMonth(tc.partitionName)
			if tc.expectErr {
				if parseErr == nil {
					t.Errorf("expected error parsing %q, got nil", tc.partitionName)
				}
				return
			}
			if parseErr != nil {
				t.Fatalf("unexpected error parsing %q: %v", tc.partitionName, parseErr)
				return
			}
			if parsed.Year() != tc.expectedYear || int(parsed.Month()) != tc.expectedMonth {
				t.Errorf("partitionMonth(%q) = %d-%02d, want %d-%02d",
					tc.partitionName, parsed.Year(), int(parsed.Month()),
					tc.expectedYear, tc.expectedMonth)
			}
		})
	}

	// Non-shadowed test for correct parsing of a known partition.
	parsed, err := partitionMonth("np_auditlog_events_2024_03")
	if err != nil {
		t.Fatalf("expected no error parsing valid partition, got: %v", err)
	}
	if parsed.Year() != 2024 || int(parsed.Month()) != 3 {
		t.Errorf("parsed = %v, want 2024-03", parsed)
	}

	// Verify that a partition end is calculated correctly (first day of next month).
	partitionEnd := parsed.AddDate(0, 1, 0)
	if partitionEnd.Year() != 2024 || int(partitionEnd.Month()) != 4 {
		t.Errorf("partition end = %v, want 2024-04-01", partitionEnd)
	}
}
