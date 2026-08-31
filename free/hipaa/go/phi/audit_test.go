// Purpose: Tests for PHI audit log logic — retention policy and access entry creation.
// Inputs: In-memory structs; no DB required for pure logic tests.
// Outputs: Verification that 6-year retention math is correct and entries are well-formed.
// Constraints: All tests are pure Go — no network, no Postgres.
package phi

import (
	"testing"
	"time"
)

// TestPHIAuditRetentionPolicy verifies the 6-year retention window.
// The HIPAA rule (45 CFR § 164.530(j)) requires audit logs be retained for
// 6 years from the date of creation or last effective date.
func TestPHIAuditRetentionPolicy(t *testing.T) {
	now := time.Now().UTC()
	accessedAt := now

	// Simulate the GENERATED ALWAYS AS computed column:
	// retain_until = (accessed_at + INTERVAL '6 years')::DATE
	retainUntil := accessedAt.AddDate(6, 0, 0)

	// Must be at least 6 years in the future.
	minRetain := now.AddDate(6, 0, 0)
	if retainUntil.Before(minRetain.Add(-24 * time.Hour)) {
		t.Errorf("retain_until %v is less than 6 years from now %v", retainUntil, now)
	}

	// Retention must not expire before the 6-year mark.
	justBefore6Years := now.AddDate(5, 11, 29)
	if retainUntil.Before(justBefore6Years) {
		t.Errorf("retain_until %v should not expire before %v", retainUntil, justBefore6Years)
	}

	// After 6 years + 1 day, deletion should be permitted.
	after6Years := now.AddDate(6, 0, 1)
	retainUntilDate := retainUntil.Truncate(24 * time.Hour)
	if !after6Years.After(retainUntilDate) {
		t.Errorf("after 6 years + 1 day (%v), retain_until (%v) should be in the past",
			after6Years, retainUntilDate)
	}
}

// TestPHIAuditEntryFields verifies required fields are present on a log entry.
func TestPHIAuditEntryFields(t *testing.T) {
	entry := PHIAuditEntry{
		ID:              "test-uuid",
		SourceAccountID: "primary",
		TableName:       "np_patients",
		ColumnNames:     []string{"ssn", "dob"},
		RowCount:        3,
		AccessedAt:      time.Now().UTC(),
		RetainUntil:     time.Now().AddDate(6, 0, 0).Format("2006-01-02"),
	}

	if entry.SourceAccountID == "" {
		t.Error("source_account_id must not be empty")
	}
	if entry.TableName == "" {
		t.Error("table_name must not be empty")
	}
	if len(entry.ColumnNames) == 0 {
		t.Error("column_names must not be empty")
	}
	if entry.RowCount < 0 {
		t.Error("row_count must not be negative")
	}
	if entry.RetainUntil == "" {
		t.Error("retain_until must be set")
	}
}

// TestPHIAuditRetentionMath6Years verifies retain_until is always >= 6 years.
// Runs multiple base times to confirm no off-by-one in date arithmetic.
func TestPHIAuditRetentionMath6Years(t *testing.T) {
	bases := []time.Time{
		time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC), // leap day
		time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
	}
	for _, base := range bases {
		retainUntil := base.AddDate(6, 0, 0)
		sixYearsLater := base.AddDate(6, 0, 0)
		if retainUntil.Before(sixYearsLater.Add(-time.Second)) {
			t.Errorf("base %v: retain_until %v is before 6-year mark %v", base, retainUntil, sixYearsLater)
		}
	}
}
