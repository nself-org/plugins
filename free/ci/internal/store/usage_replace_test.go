package store

import (
	"context"
	"testing"
)

func TestReplaceUsageAtomicAndScoped(t *testing.T) {
	for _, tc := range []struct {
		name string
		bad  bool
	}{{"successful", false}, {"key conflict rolls back", true}} {
		t.Run(tc.name, func(t *testing.T) {
			s, ctx := testStore(t), context.Background()
			row := func(key, provider, day string) UsageRow {
				return UsageRow{Key: key, Provider: provider, Day: day, Period: day[:7], CostMicro: 100}
			}
			for _, r := range []UsageRow{row("old", "github", "2026-10-20"), row("outside", "github", "2026-09-20"), row("foreign", "other", "2026-10-20")} {
				if err := s.PutUsage(ctx, r); err != nil {
					t.Fatal(err)
				}
			}
			rows := []UsageRow{row("new", "github", "2026-10-20")}
			if tc.bad {
				rows = append(rows, row("foreign", "github", "2026-10-20"))
			}
			err := s.ReplaceUsage(ctx, "github", "2026-10-01", "2026-10-31", rows)
			if (err != nil) != tc.bad {
				t.Fatalf("err=%v", err)
			}
			for _, key := range []string{"old", "new", "outside", "foreign"} {
				var n int
				if err := s.readers.QueryRowContext(ctx, "SELECT count(*) FROM ledger_usage WHERE key=?", key).Scan(&n); err != nil {
					t.Fatal(err)
				}
				want := 1
				if key == "old" && !tc.bad || key == "new" && tc.bad {
					want = 0
				}
				if n != want {
					t.Fatalf("key=%s count=%d want=%d", key, n, want)
				}
			}
		})
	}
}
