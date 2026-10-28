package store

import (
	"context"
	"testing"
)

func TestLedgerWritersRejectNegativeValues(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	checks := map[string]func() error{
		"estimate": func() error {
			return st.LedgerWrite(ctx, func(tx LedgerTx) error { return tx.PutLedger(LedgerRow{Attempt: "a", EstMicro: -1}) })
		},
		"duration": func() error {
			return st.LedgerWrite(ctx, func(tx LedgerTx) error { return tx.PutLedger(LedgerRow{Attempt: "a", EstMs: -1}) })
		},
		"rate": func() error {
			return st.LedgerWrite(ctx, func(tx LedgerTx) error { return tx.PutLedger(LedgerRow{Attempt: "a", Rate: RateSnap{ExtrasMicro: -1}}) })
		},
		"settle money":    func() error { _, err := st.LedgerSettle(ctx, "a", 0, -1, "known"); return err },
		"settle duration": func() error { _, err := st.LedgerSettle(ctx, "a", -1, 0, "known"); return err },
		"resource": func() error {
			return st.LedgerWrite(ctx, func(tx LedgerTx) error { return tx.PutResource(ResourceRow{Intent: "i", ReservedMicro: -1}) })
		},
		"resource rate": func() error {
			return st.LedgerWrite(ctx, func(tx LedgerTx) error { return tx.PutResource(ResourceRow{Intent: "i", Rate: RateSnap{MinMicro: -1}}) })
		},
		"release": func() error {
			return st.LedgerWrite(ctx, func(tx LedgerTx) error { return tx.ReleaseResource("i", 0, -1) })
		},
		"usage money":   func() error { return st.PutUsage(ctx, UsageRow{Key: "k", CostMicro: -1}) },
		"usage minutes": func() error { return st.PutUsage(ctx, UsageRow{Key: "k", Minutes: -1}) },
	}
	for name, check := range checks {
		t.Run(name, func(t *testing.T) {
			if err := check(); err == nil {
				t.Fatal("negative value accepted")
			}
		})
	}
	if rows, err := st.LedgerRecent(ctx, 10); err != nil || len(rows) != 0 {
		t.Fatalf("refusals wrote rows: %v %v", rows, err)
	}
}

func TestRateSnapshotRejectsEachNegativeField(t *testing.T) {
	for _, r := range []RateSnap{{RateMicro: -1}, {RateUnitMs: -1}, {IncMs: -1}, {MinMicro: -1}, {ExtrasMicro: -1}} {
		if err := r.validate(); err == nil {
			t.Errorf("accepted %+v", r)
		}
	}
	if err := (RateSnap{}).validate(); err != nil {
		t.Fatal(err)
	}
}
