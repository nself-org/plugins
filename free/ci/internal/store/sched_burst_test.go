package store

import (
	"context"
	"errors"
	"testing"
)

func burstIntent(token string) (BurstIntent, BurstLease) {
	return BurstIntent{Token: token, Coordinator: "coord-1", Provider: "cloud", Pipeline: "pipe", JobKey: "job-" + token, Platform: "linux/amd64", ReservedMicro: 100, LifetimeMs: 6_300_000, CreatedAt: 1000},
		BurstLease{IdleUntil: 5000, MaxUntil: 9000}
}

func TestBurstStoreIntentStateMovesForwardOnly(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	in, lease := burstIntent("T1")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.BurstWrite(ctx, func(b BurstTx) error { return b.PutIntent(in, lease) }))
	if err := s.BurstWrite(ctx, func(b BurstTx) error { return b.PutIntent(in, lease) }); err == nil {
		t.Fatal("the token is unique: a second intent with it must fail")
	}
	got, ok, err := s.BurstIntent(ctx, "T1")
	if err != nil || !ok || got.State != BurstLive || got.Coordinator != "coord-1" || got.ReservedMicro != 100 || got.ResourceID != "" {
		t.Fatalf("%+v %v %v", got, ok, err)
	}
	if l, ok, _ := s.BurstLease(ctx, "T1"); !ok || l != lease {
		t.Fatalf("lease %+v", l)
	}
	must(s.BurstWrite(ctx, func(b BurstTx) error {
		if err := b.SetResource("T1", "res-1", 2000); err != nil {
			return err
		}
		if bound, err := b.SetNode("T1", "node-1", 2100); err != nil || !bound {
			t.Errorf("bind %v %v", bound, err)
		}
		if bound, _ := b.SetNode("T1", "node-2", 2200); bound {
			t.Error("a node is bound once")
		}
		return b.SetIdle("T1", 7777)
	}))
	must(s.BurstWrite(ctx, func(b BurstTx) error { return b.MarkTearingDown("T1", "job done", 3000) }))
	must(s.BurstWrite(ctx, func(b BurstTx) error { return b.MarkTearingDown("T1", "again", 3100) }))
	got, _, _ = s.BurstIntent(ctx, "T1")
	if got.State != BurstTearingDown || got.Reason != "job done" || got.ResourceID != "res-1" || got.NodeID != "node-1" {
		t.Fatalf("a second mark must not rewrite the first reason: %+v", got)
	}
	must(s.BurstWrite(ctx, func(b BurstTx) error { return b.MarkTerminal("T1", 4000) }))
	must(s.BurstWrite(ctx, func(b BurstTx) error { return b.MarkTearingDown("T1", "late", 4100) }))
	got, _, _ = s.BurstIntent(ctx, "T1")
	if got.State != BurstTerminal {
		t.Fatalf("terminal is final: %+v", got)
	}
	if _, ok, _ := s.BurstLease(ctx, "T1"); ok {
		t.Fatal("a terminal intent has no lease")
	}
	if open, _ := s.BurstIntents(ctx, "coord-1"); len(open) != 0 {
		t.Fatalf("terminal intents are not listed: %+v", open)
	}
	if err = s.BurstWrite(ctx, func(b BurstTx) error {
		_, err := b.tx.ExecContext(ctx, `UPDATE burst_intent SET state='bogus' WHERE token='T1'`)
		return err
	}); err == nil {
		t.Fatal("the state column is constrained")
	}
}

func TestBurstStoreNodeUseIsOncePerNodeAndRollsBack(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	boom := errors.New("later step refused")
	err := s.BurstWrite(ctx, func(b BurstTx) error {
		if first, err := b.NodeUse("node-1", "T1", "a1", 10); err != nil || !first {
			t.Errorf("first use: %v %v", first, err)
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if used, _ := s.BurstNodeUsed(ctx, "node-1"); used {
		t.Fatal("a rolled-back admission must leave no claim on the node")
	}
	for i, want := range []bool{true, false, false} {
		_ = s.BurstWrite(ctx, func(b BurstTx) error {
			first, err := b.NodeUse("node-1", "T1", "a"+string(rune('1'+i)), 10)
			if first != want || err != nil {
				t.Errorf("use %d: first=%v err=%v, want %v", i, first, err, want)
			}
			return nil
		})
	}
}

func TestBurstStoreBreakerAndTeardownFailure(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for i := 1; i <= 3; i++ {
		_ = s.BurstWrite(ctx, func(b BurstTx) error {
			st, err := b.BreakerFail("cloud", 3, 900_000, 1000)
			if err != nil || st.Failures != i || (i < 3) != (st.OpenUntil == 0) {
				t.Errorf("failure %d: %+v %v", i, st, err)
			}
			return nil
		})
	}
	if b, _ := s.Breaker(ctx, "cloud"); b.Failures != 3 || b.OpenUntil != 901_000 {
		t.Fatalf("%+v", b)
	}
	_ = s.BurstWrite(ctx, func(b BurstTx) error { return b.BreakerReset("cloud") })
	if b, _ := s.Breaker(ctx, "cloud"); b.Failures != 0 || b.OpenUntil != 0 {
		t.Fatalf("%+v", b)
	}
	alerts := 0
	for i := 0; i < 5; i++ {
		_ = s.BurstWrite(ctx, func(b BurstTx) error {
			a, err := b.TeardownFailed("T9", "cloud", int64(100+i))
			if err != nil {
				t.Error(err)
			}
			if a {
				alerts++
			}
			return nil
		})
	}
	f, ok, _ := s.TeardownFailure(ctx, "T9")
	if alerts != 1 || !ok || f.FirstFailedAt != 100 || f.Attempts != 5 || !f.Alerted {
		t.Fatalf("alerts %d, %+v", alerts, f)
	}
	c, _ := s.BurstCountsOf(ctx, "cloud", "job", 0)
	if c.TeardownFailed != 1 {
		t.Fatalf("%+v", c)
	}
}

func TestBurstStoreCounts(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for i, tok := range []string{"A", "B", "C", "D"} {
		in, lease := burstIntent(tok)
		in.JobKey, in.CreatedAt = "job-1", int64(1000*(i+1))
		_ = s.BurstWrite(ctx, func(b BurstTx) error { return b.PutIntent(in, lease) })
	}
	_ = s.BurstWrite(ctx, func(b BurstTx) error { _, err := b.SetNode("B", "n", 1); return err })
	_ = s.BurstWrite(ctx, func(b BurstTx) error { return b.MarkTerminal("C", 1) })
	c, err := s.BurstCountsOf(ctx, "cloud", "job-1", 2000)
	if err != nil || c.InFlight != 2 || c.Open != 3 || c.JobWindow != 3 {
		t.Fatalf("%+v %v (A and D in flight, B joined, C terminal; window counts every intent from 2000, terminal included)", c, err)
	}
	if p, _ := s.BurstPending(ctx); len(p) != 2 || p[0] != [2]string{"cloud", "linux/amd64"} {
		t.Fatalf("%v", p)
	}
}

func TestBurstStoreRevokeHookMarksTheNodesIntentInTheRevokeTransaction(t *testing.T) {
	s, n := lifecycleStore(t)
	ctx := context.Background()
	admin, _ := NewAdminAuthority("cli:test")
	if err := s.NodeAdd(ctx, n, admin); err != nil {
		t.Fatal(err)
	}
	in, lease := burstIntent("T1")
	other, _ := burstIntent("T2")
	_ = s.BurstWrite(ctx, func(b BurstTx) error {
		if err := b.PutIntent(in, lease); err != nil {
			return err
		}
		if err := b.PutIntent(other, lease); err != nil {
			return err
		}
		_, err := b.SetNode("T1", n.ID, 1)
		return err
	})
	boom := errors.New("deny list down")
	_, err := s.NodeRevoke(ctx, n.ID, "cli:test", []RevokeHook{BurstRevokeHook(), func(context.Context, *RevokeTx, string) error { return boom }})
	if err == nil {
		t.Fatal("the failing hook must abort the revoke")
	}
	if got, _, _ := s.BurstIntent(ctx, "T1"); got.State != BurstLive {
		t.Fatalf("revoke and the intent mark are one transaction: a rolled-back revoke leaves the intent live: %+v", got)
	}
	if nr, _ := s.NodeGet(ctx, n.ID); nr.State == "revoked" {
		t.Fatal("the node must not be revoked after a failed hook")
	}
	if _, err = s.NodeRevoke(ctx, n.ID, "cli:test", []RevokeHook{BurstRevokeHook()}); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := s.BurstIntent(ctx, "T1"); got.State != BurstTearingDown {
		t.Fatalf("%+v", got)
	}
	if got, _, _ := s.BurstIntent(ctx, "T2"); got.State != BurstLive {
		t.Fatalf("another node's intent must not move: %+v", got)
	}
}
