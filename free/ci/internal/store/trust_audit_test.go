package store

// Purpose: tests of the chained-audit store functions with a stand-in chain function (the real hash lives
// in internal/audit): sealing order and links, the partial index, rollback with the caller's transaction.
// Inputs:  testStore. Outputs: pass or fail.
// Constraints: no hashing here; the stand-in head is "h<seq>" so a wrong link is visible.

import (
	"context"
	"fmt"
	"testing"
)

func standIn(prev []byte, r AuditRow) (AuditChain, error) {
	return AuditChain{Head: []byte(fmt.Sprintf("h%d", r.Seq))}, nil
}

func TestAuditSealOrderAndLinks(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := s.AppendAudit(ctx, "a", "x", fmt.Sprint("t", i), "d"); err != nil {
			t.Fatal(err)
		}
	}
	if n, ts, err := s.AuditPending(ctx); err != nil || n != 3 || ts == 0 {
		t.Fatalf("pending = %d %d %v", n, ts, err)
	}
	if n, err := s.AuditSeal(ctx, standIn); err != nil || n != 3 {
		t.Fatalf("seal = %d %v", n, err)
	}
	rows, _ := s.AuditEntries(ctx, 0, 0)
	if string(rows[0].PrevHead) != "" || string(rows[1].PrevHead) != "h1" || string(rows[2].PrevHead) != "h2" || string(rows[2].Head) != "h3" {
		t.Fatalf("links wrong: %q %q %q", rows[0].PrevHead, rows[1].PrevHead, rows[2].PrevHead)
	}
	if n, err := s.AuditSeal(ctx, standIn); err != nil || n != 0 {
		t.Fatalf("a second seal is a no-op: %d %v", n, err)
	}
	if _, err := s.AuditSeal(ctx, nil); err == nil {
		t.Fatal("no chain function is an error")
	}
	if err := s.AuditAppend(ctx, AuditInput{Actor: "a", Action: "x", Target: "t", Detail: "secret=oops", Seal: standIn}); err == nil {
		t.Fatal("the core row check still applies")
	}
}

func TestAuditAppendInPolicyTxRollsBack(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	w := PolicyWrite{Op: PolicySet, Scope: "project", Subject: "proj", Key: "trust.network", Value: []byte(`"none"`), Actor: "ali"}
	err := s.ApplyWrite(ctx, w, func(tx *Tx) error {
		if err := tx.AuditAppend(AuditInput{Actor: "ali", Action: "policy.set", Target: "proj", Detail: "d", Seal: standIn}); err != nil {
			return err
		}
		return fmt.Errorf("boom")
	})
	if err == nil || auditCount(t, s) != 0 {
		t.Fatalf("rolled back with the write: %v, rows=%d", err, auditCount(t, s))
	}
}
