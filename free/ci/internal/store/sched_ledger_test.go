package store

// Tests of the usage ledger store (P7-SCHED-07): the day-window high-water
// mark, reservations that are written once and summed without wrapping, settle
// charging the admission window, provider ownership, approvals bound to the
// pipeline row, and provider usage as the allowance input.

import (
	"context"
	"math"
	"testing"
)

func TestLedgerWindowKeepsTheHighWaterMark(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for _, tc := range []struct{ today, want string }{
		{"2026-10-31", "2026-10-31"}, {"2026-11-01", "2026-11-01"}, {"2026-10-31", "2026-11-01"}, {"2026-11-01", "2026-11-01"}, {"2026-11-02", "2026-11-02"},
	} {
		var got string
		if err := s.LedgerWrite(ctx, func(tx LedgerTx) (err error) { got, err = tx.Window(tc.today); return }); err != nil || got != tc.want {
			t.Fatalf("Window(%s) = %q, %v; want %q", tc.today, got, err, tc.want)
		}
	}
	if high, _ := s.WindowHigh(ctx, "2026-11-02"); high != "2026-11-02" {
		t.Fatalf("high %s", high)
	}
	for _, bad := range []string{"", "2026-13-01", "11/01/2026", "2026-11-1x"} {
		if err := s.LedgerWrite(ctx, func(tx LedgerTx) error { _, err := tx.Window(bad); return err }); err == nil {
			t.Errorf("accepted the window key %q", bad)
		}
	}
}

func TestLedgerReservationsAreWrittenOnceAndSumWithoutWrapping(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	err := s.LedgerWrite(ctx, func(tx LedgerTx) error {
		for i := 0; i < 3; i++ { // a repeat of the same admission never double-counts
			if err := tx.Reserve(Reservation{Owner: "a1", Scope: ScopeDay, WindowKey: "2026-10-19", Micro: 100}); err != nil {
				return err
			}
		}
		return tx.Reserve(Reservation{Owner: "a2", Scope: ScopeDay, WindowKey: "2026-10-19", Micro: 50})
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Reserved(ctx, ScopeDay, "", "2026-10-19"); got != 150 {
		t.Fatalf("held %d", got)
	}
	if got, _ := s.Reserved(ctx, ScopeDay, "", "2026-10-20"); got != 0 {
		t.Fatalf("another window: %d", got)
	}
	_ = s.LedgerWrite(ctx, func(tx LedgerTx) error {
		_ = tx.Reserve(Reservation{Owner: "big1", Scope: ScopeMonth, WindowKey: "2026-10", Micro: math.MaxInt64 - 5})
		return tx.Reserve(Reservation{Owner: "big2", Scope: ScopeMonth, WindowKey: "2026-10", Micro: math.MaxInt64 - 5})
	})
	if got, err := s.Reserved(ctx, ScopeMonth, "", "2026-10"); err != nil || got != math.MaxInt64 {
		t.Fatalf("a sum past int64 saturates, never wraps or errors: %d %v", got, err)
	}
}

func TestLedgerSettleOnceChargesTheAdmissionWindow(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	err := s.LedgerWrite(ctx, func(tx LedgerTx) error {
		if err := tx.PutLedger(LedgerRow{Attempt: "a1", Pipeline: "p", Provider: "x", Class: "metered", EstMicro: 100, Confidence: "estimated", WindowDay: "2026-10-31"}); err != nil {
			return err
		}
		return tx.Reserve(Reservation{Owner: "a1", Scope: ScopeMonth, WindowKey: "2026-10", Micro: 100})
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := s.LedgerSettle(ctx, "a1", 5, 30, "computed"); err != nil || !ok {
		t.Fatalf("settle: %v %v", ok, err)
	}
	if ok, _ := s.LedgerSettle(ctx, "a1", 9, 90, "computed"); ok {
		t.Fatal("settled twice")
	}
	if ok, _ := s.LedgerSettle(ctx, "no-such", 1, 1, "computed"); ok {
		t.Fatal("settled an attempt with no row")
	}
	if got, _ := s.Reserved(ctx, ScopeMonth, "", "2026-10"); got != 30 {
		t.Fatalf("october holds %d, want the actual 30", got)
	}
	row, ok, _ := s.LedgerAttempt(ctx, "a1")
	if !ok || row.ActMicro == nil || *row.ActMicro != 30 || row.Confidence != "computed" {
		t.Fatalf("%+v", row)
	}
	// An actual above the reservation is clamped to what was reserved.
	_ = s.LedgerWrite(ctx, func(tx LedgerTx) error {
		_ = tx.PutLedger(LedgerRow{Attempt: "a2", Pipeline: "p", Provider: "x", Class: "metered", EstMicro: 10, Confidence: "estimated", WindowDay: "2026-10-31"})
		return tx.Reserve(Reservation{Owner: "a2", Scope: ScopeMonth, WindowKey: "2026-11", Micro: 10})
	})
	_, _ = s.LedgerSettle(ctx, "a2", 5, 999, "computed")
	if got, _ := s.Reserved(ctx, ScopeMonth, "", "2026-11"); got != 10 {
		t.Fatalf("an actual cannot exceed its reservation: %d", got)
	}
}

func TestLedgerProviderOwnerIsClaimedOnce(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	var first, second, peek string
	_ = s.LedgerWrite(ctx, func(tx LedgerTx) (err error) {
		if peek, err = tx.ProviderOwner("cloud", ""); err != nil {
			return err
		}
		if first, err = tx.ProviderOwner("cloud", "coord-a"); err != nil {
			return err
		}
		second, err = tx.ProviderOwner("cloud", "coord-b")
		return err
	})
	if peek != "" || first != "coord-a" || second != "coord-a" {
		t.Fatalf("peek %q first %q second %q", peek, first, second)
	}
	if owners, _ := s.ProviderOwners(ctx); owners["cloud"] != "coord-a" {
		t.Fatalf("%v", owners)
	}
}

func TestLedgerApprovalIsBoundToThePipelineRow(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	admitSeed(t, s, "pipe", "proj", "job", "a1")
	facts, ok, err := s.PipelineFacts(ctx, "pipe")
	if err != nil || !ok || facts.Revision != "rev" || facts.ConfigDigest != "i" || facts.Project != "proj" {
		t.Fatalf("facts: %+v %v %v", facts, ok, err)
	}
	if err = s.PutApproval(ctx, Approval{Run: "pipe", SHA: "rev", ConfigDigest: "i", EstimateMicro: 77, Actor: "user:adm"}, "amount_micro=77 sha=rev"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.ValidApprovals(ctx); v["pipe"] != 77 {
		t.Fatalf("%v", v)
	}
	if err = s.PutApproval(ctx, Approval{Run: "pipe", SHA: "other", ConfigDigest: "i", EstimateMicro: 77, Actor: "user:adm"}, "x"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.ValidApprovals(ctx); len(v) != 0 {
		t.Fatalf("an approval for another SHA counts: %v", v)
	}
	if s.PutApproval(ctx, Approval{Run: "", SHA: "rev", Actor: "a"}, "x") == nil || s.PutApproval(ctx, Approval{Run: "pipe", SHA: "rev", Actor: ""}, "x") == nil {
		t.Fatal("an incomplete approval was stored")
	}
	rows, _ := s.AuditEntries(ctx, 0, 0)
	var n int
	for _, r := range rows {
		if r.Action == "spend.approved" {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("every stored approval is audited in the same commit: %d rows", n)
	}
}

func TestLedgerAllowanceInputsAndPending(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	bal := int64(900)
	for i, u := range []UsageRow{{Key: "k1", Provider: "gh", Period: "2026-10", Minutes: 1, CostMicro: 100, ReportedAt: 10}, {Key: "k2", Provider: "gh", Period: "2026-10", Minutes: 1, CostMicro: 50, BalanceMicro: &bal, ReportedAt: 20},
		{Key: "k1", Provider: "gh", Period: "2026-10", Minutes: 1, CostMicro: 100, ReportedAt: 10}, {Key: "k3", Provider: "gh", Period: "2026-09", CostMicro: 7, ReportedAt: 5}} {
		if err := s.PutUsage(ctx, u); err != nil {
			t.Fatal(i, err)
		}
	}
	in, err := s.Allowance(ctx, "gh", "2026-10")
	if err != nil || !in.HasImport || in.Imported != 150 || in.Balance == nil || *in.Balance != 900 {
		t.Fatalf("%+v %v", in, err)
	}
	if in, _ = s.Allowance(ctx, "gh", "2026-08"); in.HasImport {
		t.Fatalf("no rows that month: %+v", in)
	}
	if err = s.NotePending(ctx, "r1", 500); err != nil || s.NotePending(ctx, "r1", 200) != nil {
		t.Fatal(err)
	}
	noted, details, err := s.PendingEstimate(ctx, "r1")
	if err != nil || noted != 500 || len(details) != 0 {
		t.Fatalf("%d %v %v", noted, details, err)
	}
	if s.NotePending(ctx, "", 1) == nil || s.NotePending(ctx, "r", -1) == nil {
		t.Fatal("accepted a bad pending estimate")
	}
}

// TestLedgerSettledRowsAreFinal: re-admission of an attempt replaces its open
// estimate and reservation, never a settled one; and PipelineHeld leaves out
// the attempt being admitted.
func TestLedgerSettledRowsAreFinal(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	put := func(est int64) error {
		return s.LedgerWrite(ctx, func(tx LedgerTx) error {
			if err := tx.PutLedger(LedgerRow{Attempt: "a1", Pipeline: "p", Provider: "x", Class: "metered", EstMicro: est, Confidence: "estimated", WindowDay: "2026-10-19"}); err != nil {
				return err
			}
			return tx.Reserve(Reservation{Owner: "a1", Scope: ScopeDay, WindowKey: "2026-10-19", Micro: est})
		})
	}
	if err := put(900); err != nil {
		t.Fatal(err)
	}
	if err := put(500); err != nil { // an open one is replaced
		t.Fatal(err)
	}
	if row, _, _ := s.LedgerAttempt(ctx, "a1"); row.EstMicro != 500 {
		t.Fatalf("open row not replaced: %+v", row)
	}
	if held, _ := s.Reserved(ctx, ScopeDay, "", "2026-10-19"); held != 500 {
		t.Fatalf("open reservation not replaced: %d", held)
	}
	if ok, err := s.LedgerSettle(ctx, "a1", 1, 40, "computed"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := put(999); err != nil { // a settled one is final
		t.Fatal(err)
	}
	row, _, _ := s.LedgerAttempt(ctx, "a1")
	if row.EstMicro != 500 || row.ActMicro == nil || *row.ActMicro != 40 || row.FinalizedAt == 0 {
		t.Fatalf("a settled ledger row was overwritten: %+v", row)
	}
	if held, _ := s.Reserved(ctx, ScopeDay, "", "2026-10-19"); held != 40 {
		t.Fatalf("a settled reservation was overwritten: %d", held)
	}
	_ = s.LedgerWrite(ctx, func(tx LedgerTx) error {
		if got, _ := tx.PipelineHeld("p", "a1"); got != 0 {
			t.Errorf("an attempt counted against itself: %d", got)
		}
		if got, _ := tx.PipelineHeld("p", "other"); got != 40 {
			t.Errorf("a settled attempt counts at its actual: %d", got)
		}
		return nil
	})
}

// TestLedgerWindowMarkFromAJumpedClockIsReplaced: a recorded day far past today (a clock
// that jumped forward once) or one that is not a date is replaced by today and audited; a
// clock stepped back by a day is the case the mark exists for and is kept.
func TestLedgerWindowMarkFromAJumpedClockIsReplaced(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	win := func(today string) string {
		var got string
		if err := s.LedgerWrite(ctx, func(tx LedgerTx) (err error) { got, err = tx.Window(today); return }); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := win("2026-11-02"); got != "2026-11-02" {
		t.Fatal(got)
	}
	if got := win("2026-10-31"); got != "2026-11-02" { // two days back: kept
		t.Fatalf("a short step back must keep the later window, got %s", got)
	}
	if got := win("2026-10-20"); got != "2026-10-20" { // 13 days back: the mark is the anomaly
		t.Fatalf("a mark far ahead of today must be replaced, got %s", got)
	}
	// A damaged row is replaced too, and never panics a month slice.
	if err := s.LedgerWrite(ctx, func(tx LedgerTx) error {
		_, err := tx.tx.ExecContext(ctx, "UPDATE window_high SET key='x' WHERE kind='day'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if high, _ := s.WindowHigh(ctx, "2026-10-20"); high != "" {
		t.Fatalf("a damaged mark is no mark: %q", high)
	}
	if got := win("2026-10-21"); got != "2026-10-21" {
		t.Fatal(got)
	}
	rows, _ := s.AuditEntries(ctx, 0, 0)
	var n int
	for _, r := range rows {
		if r.Action == "ledger.window.reset" {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("each replacement is audited: %d rows", n)
	}
}

func TestLedgerResourceReleaseKeepsTheRealCost(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	_ = s.LedgerWrite(ctx, func(tx LedgerTx) error {
		if err := tx.PutResource(ResourceRow{Intent: "vm", Provider: "x", Pipeline: "p", WindowDay: "2026-10-19", ReservedMicro: 100, ProvisionedAt: 1}); err != nil {
			return err
		}
		return tx.Reserve(Reservation{Owner: "vm", Scope: ScopeDay, WindowKey: "2026-10-19", Micro: 100})
	})
	_ = s.LedgerWrite(ctx, func(tx LedgerTx) error { return tx.ReleaseResource("vm", 2, 5000) })
	if held, _ := s.Reserved(ctx, ScopeDay, "", "2026-10-19"); held != 5000 {
		t.Fatalf("a VM that outlived its reservation cost 5000, the day holds %d", held)
	}
	row, _, _ := s.Resource(ctx, "vm")
	if row.ActualMicro == nil || *row.ActualMicro != 5000 {
		t.Fatalf("%+v", row)
	}
}
