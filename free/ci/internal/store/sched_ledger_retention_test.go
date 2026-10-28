package store

// Tests of the usage ledger retention (D-1281): settled rows of windows older
// than LedgerKeepDays go when the next row is inserted; every row a budget
// check can still read stays.

import (
	"context"
	"testing"
	"time"
)

func TestLedgerRetentionPrunesSettledRowsOfPastWindows(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	clock := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return clock }
	old, recent := "2025-01-01", "2026-10-01"
	seedWindow := func(day string) {
		err := s.LedgerWrite(ctx, func(tx LedgerTx) error {
			for _, id := range []string{"set-" + day, "open-" + day} {
				if err := tx.PutLedger(LedgerRow{Attempt: id, Pipeline: "p", Provider: "x", Class: "metered", EstMicro: 10, Confidence: "estimated", WindowDay: day}); err != nil {
					return err
				}
				if err := tx.Reserve(Reservation{Owner: id, Scope: ScopeDay, WindowKey: day, Micro: 10}); err != nil {
					return err
				}
			}
			if err := tx.Reserve(Reservation{Owner: "set-" + day, Scope: ScopeMonth, WindowKey: day[:7], Micro: 10}); err != nil {
				return err
			}
			if err := tx.Reserve(Reservation{Owner: "set-" + day, Scope: ScopeProvider, ScopeKey: "x", Micro: 10}); err != nil {
				return err
			}
			if err := tx.PutResource(ResourceRow{Intent: "done-" + day, Provider: "x", Pipeline: "p", WindowDay: day, ReservedMicro: 10, ProvisionedAt: 1}); err != nil {
				return err
			}
			return tx.PutResource(ResourceRow{Intent: "vm-" + day, Provider: "x", Pipeline: "p", WindowDay: day, ReservedMicro: 10, ProvisionedAt: 1})
		})
		if err != nil {
			t.Fatal(err)
		}
		if ok, err := s.LedgerSettle(ctx, "set-"+day, 5, 5, "known"); err != nil || !ok {
			t.Fatal(ok, err)
		}
		if err = s.LedgerWrite(ctx, func(tx LedgerTx) error { return tx.ReleaseResource("done-"+day, 2, 5) }); err != nil {
			t.Fatal(err)
		}
		if err = s.PutUsage(ctx, UsageRow{Key: "u-" + day, Provider: "x", Period: day[:7], ReportedAt: 1}); err != nil {
			t.Fatal(err)
		}
	}
	seedWindow(old)
	seedWindow(recent)
	count := func(q string) (n int) {
		if err := s.readers.QueryRowContext(ctx, q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return
	}
	if n := count("SELECT count(*) FROM ledger"); n != 4 {
		t.Fatalf("seed: %d ledger rows", n)
	}
	clock = time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	if err := s.LedgerWrite(ctx, func(tx LedgerTx) error {
		return tx.PutLedger(LedgerRow{Attempt: "new", Pipeline: "p", Provider: "x", Class: "metered", EstMicro: 10, Confidence: "estimated", WindowDay: "2026-10-20"})
	}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, q string
		want    int
	}{
		{"settled ledger row of the old window", "SELECT count(*) FROM ledger WHERE attempt_id='set-" + old + "'", 0},
		{"open ledger row of the old window", "SELECT count(*) FROM ledger WHERE attempt_id='open-" + old + "'", 1},
		{"settled ledger row of a recent window", "SELECT count(*) FROM ledger WHERE attempt_id='set-" + recent + "'", 1},
		{"settled day and month reservations of the old window", "SELECT count(*) FROM reservation WHERE settled=1 AND window_key LIKE '2025-01%'", 0},
		{"unsettled reservation of the old window", "SELECT count(*) FROM reservation WHERE settled=0 AND window_key='" + old + "'", 1},
		{"settled reservations of a recent window", "SELECT count(*) FROM reservation WHERE settled=1 AND window_key LIKE '2026-10%'", 2},
		{"provider-scope reservation (no window)", "SELECT count(*) FROM reservation WHERE scope='provider'", 2},
		{"torn-down resource of the old window", "SELECT count(*) FROM resource_ledger WHERE intent='done-" + old + "'", 0},
		{"resource still holding money, old window", "SELECT count(*) FROM resource_ledger WHERE intent='vm-" + old + "'", 1},
		{"torn-down resource of a recent window", "SELECT count(*) FROM resource_ledger WHERE intent='done-" + recent + "'", 1},
		{"provider usage of the old month", "SELECT count(*) FROM ledger_usage WHERE period='2025-01'", 0},
		{"provider usage of a recent month", "SELECT count(*) FROM ledger_usage WHERE period='2026-10'", 1},
	} {
		if got := count(c.q); got != c.want {
			t.Errorf("%s: %d rows, want %d", c.name, got, c.want)
		}
	}
}
