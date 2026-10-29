package store

import (
	"context"
	"testing"
)

// ReplaceUsage swaps the days of one provider atomically and leaves other days and providers alone (D-1317).
func TestReplaceUsageSwapsOneProvidersDays(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	u := func(key, provider, day string, cost int64) UsageRow {
		return UsageRow{Key: key, Provider: provider, Period: "2026-10", Day: day, CostMicro: cost, ReportedAt: 10}
	}
	for _, row := range []UsageRow{
		u("old-name", "gh", "2026-10-17", 100), u("before", "gh", "2026-10-10", 7), u("other", "elsewhere", "2026-10-17", 5),
	} {
		if err := s.PutUsage(ctx, row); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.ReplaceUsage(ctx, "gh", "2026-10-15", "2026-10-19", []UsageRow{u("new-name", "gh", "2026-10-17", 100)}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		provider string
		want     int64
	}{{"gh", 107}, {"elsewhere", 5}} {
		if in, err := s.Allowance(ctx, c.provider, "2026-10"); err != nil || in.Imported != c.want {
			t.Fatalf("%s imports %d (%v), want %d: the renamed row must replace the old one, other days and providers stay", c.provider, in.Imported, err, c.want)
		}
	}
	// A row outside the range or of another provider is refused and nothing is deleted.
	for _, bad := range []UsageRow{u("x", "gh", "2026-10-20", 1), u("y", "elsewhere", "2026-10-17", 1)} {
		if err := s.ReplaceUsage(ctx, "gh", "2026-10-15", "2026-10-19", []UsageRow{bad}); !codeIs(err, "E607") {
			t.Fatalf("row %s: %v", bad.Key, err)
		}
	}
	if in, _ := s.Allowance(ctx, "gh", "2026-10"); in.Imported != 107 {
		t.Fatalf("a refused replacement changed the ledger: %d", in.Imported)
	}
	if err := s.ReplaceUsage(ctx, "gh", "2026-10-19", "2026-10-15", nil); !codeIs(err, "E607") {
		t.Fatalf("an unordered range: %v", err)
	}
}
