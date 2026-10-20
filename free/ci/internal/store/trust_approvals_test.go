package store

// Purpose: tests of the approval rows (P7-TRUST-24): an approval binds the stored head SHA, config digest
// and head repo; a change voids it with one audit row; an approval writes one audit row; validity never
// depends on a void having been written.
// Inputs:  testStore with seeded pipeline rows, and a void check written here (the real rule lives in
//          trust/source and is tested there).
// Outputs: pass or fail.
// Constraints: no network; the check compares fields exactly.

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func exactCheck(a, cur ApprovalBinding) []string {
	var out []string
	if a.HeadSHA != cur.HeadSHA {
		out = append(out, "void.head_sha")
	}
	if a.ConfigDigest != cur.ConfigDigest {
		out = append(out, "void.config_digest")
	}
	if a.HeadRepo != cur.HeadRepo {
		out = append(out, "void.head_repo")
	}
	return out
}

func approvalSeed(t *testing.T, s *Store, run string) {
	t.Helper()
	p := PipelineRow{ID: run, Revision: "sha-1", Project: "/proj", Trigger: "push", SourceTrust: "untrusted", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "cfg-1", SelectionMode: "full", Status: "queued"}
	if err := s.CreatePipeline(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordApprovalFacts(context.Background(), ApprovalRunFacts{Run: run, Author: "mallory", HeadRepo: "fork/app", RequiresApproval: true}, exactCheck); err != nil {
		t.Fatal(err)
	}
}

func auditActions(t *testing.T, s *Store, action string) int {
	t.Helper()
	var n int
	if err := s.readers.QueryRow("SELECT count(*) FROM audit WHERE action=?", action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func moveRun(t *testing.T, s *Store, run, sha, digest string) {
	t.Helper()
	if _, err := s.writer.Exec("UPDATE pipeline SET revision=?,input_digest=? WHERE id=?", sha, digest, run); err != nil {
		t.Fatal(err)
	}
}

func TestTrustApprovalsBindAndVoid(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	approvalSeed(t, s, "r1")
	a, err := s.ApproveRun(ctx, ApproveInput{Run: "r1", Approver: "token:t1:bob", ExpectSHA: "sha-1"})
	if err != nil {
		t.Fatal(err)
	}
	want := ApprovalBinding{Run: "r1", HeadSHA: "sha-1", ConfigDigest: "cfg-1", HeadRepo: "fork/app"}
	if a.ApprovalBinding != want {
		t.Fatalf("binding %+v, want %+v", a.ApprovalBinding, want)
	}
	if auditActions(t, s, "approval.grant") != 1 {
		t.Fatal("an approval writes one audit row")
	}
	// A sweep over an unchanged run voids nothing and writes nothing.
	if v, err := s.SweepApprovals(ctx, "", exactCheck); err != nil || len(v) != 0 || auditActions(t, s, "approval.void") != 0 {
		t.Fatalf("unchanged run: %v %v", v, err)
	}
	// A push moves the head: the state reads as mismatched before any void is written.
	moveRun(t, s, "r1", "sha-2", "cfg-1")
	st, _, _ := s.ApprovalState(ctx, "r1")
	if !st.HasApproval || st.Approval.VoidedAt != 0 || len(exactCheck(st.Approval.ApprovalBinding, st.Current())) != 1 {
		t.Fatalf("mismatch must be visible without a write: %+v", st)
	}
	v, err := s.SweepApprovals(ctx, "", exactCheck)
	if err != nil || len(v) != 1 || v[0].VoidReason != "void.head_sha" {
		t.Fatalf("sweep: %+v %v", v, err)
	}
	if auditActions(t, s, "approval.void") != 1 {
		t.Fatal("a void writes one audit row")
	}
	if v, _ = s.SweepApprovals(ctx, "", exactCheck); len(v) != 0 || auditActions(t, s, "approval.void") != 1 {
		t.Fatal("a voided approval is voided once")
	}
	// A stale approver (looked at sha-1, run is at sha-2) is refused; the current head can be approved again.
	if _, err = s.ApproveRun(ctx, ApproveInput{Run: "r1", Approver: "bob", ExpectSHA: "sha-1"}); !errors.Is(err, ErrApprovalStale) {
		t.Fatalf("stale approver: %v", err)
	}
	if _, err = s.ApproveRun(ctx, ApproveInput{Run: "r1", Approver: "bob", ExpectSHA: "sha-2"}); err != nil {
		t.Fatal(err)
	}
	st, _, _ = s.ApprovalState(ctx, "r1")
	if st.Approval.VoidedAt != 0 || st.Approval.HeadSHA != "sha-2" {
		t.Fatalf("a fresh approval replaces the voided one: %+v", st.Approval)
	}
	if _, err = s.ApproveRun(ctx, ApproveInput{Run: "nope", Approver: "bob"}); !errors.Is(err, ErrApprovalUnknownRun) {
		t.Fatalf("unknown run: %v", err)
	}
	if _, err = s.ApproveRun(ctx, ApproveInput{Run: "r1"}); !codeIs(err, "E607") {
		t.Fatalf("no approver: %v", err)
	}
}

func TestTrustApprovalsFactsAndConfigVoid(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	approvalSeed(t, s, "r2")
	if _, err := s.ApproveRun(ctx, ApproveInput{Run: "r2", Approver: "bob"}); err != nil {
		t.Fatal(err)
	}
	// Re-recording the same facts keeps the approval.
	v, err := s.RecordApprovalFacts(ctx, ApprovalRunFacts{Run: "r2", Author: "mallory", HeadRepo: "fork/app", RequiresApproval: true}, exactCheck)
	if err != nil || len(v) != 0 {
		t.Fatalf("same facts: %v %v", v, err)
	}
	// A changed head repo voids it inside the facts write.
	v, err = s.RecordApprovalFacts(ctx, ApprovalRunFacts{Run: "r2", Author: "mallory", HeadRepo: "other/app", RequiresApproval: true}, exactCheck)
	if err != nil || len(v) != 1 || v[0].VoidReason != "void.head_repo" {
		t.Fatalf("head repo change: %v %v", v, err)
	}
	if _, err = s.ApproveRun(ctx, ApproveInput{Run: "r2", Approver: "bob"}); err != nil {
		t.Fatal(err)
	}
	moveRun(t, s, "r2", "sha-1", "cfg-2")
	v, _ = s.SweepApprovals(ctx, "r2", exactCheck)
	if len(v) != 1 || v[0].VoidReason != "void.config_digest" {
		t.Fatalf("config change: %v", v)
	}
	if auditActions(t, s, "approval.void") != 2 || auditActions(t, s, "approval.grant") != 2 {
		t.Fatal("one audit row per approval and per void")
	}
	// A pruned run leaves nothing to bind to: its approval voids.
	approvalSeed(t, s, "r3")
	if _, err = s.ApproveRun(ctx, ApproveInput{Run: "r3", Approver: "bob"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.writer.Exec("DELETE FROM pipeline WHERE id='r3'"); err != nil {
		t.Fatal(err)
	}
	if v, err = s.SweepApprovals(ctx, "r3", exactCheck); err != nil || len(v) != 1 {
		t.Fatalf("pruned run: %v %v", v, err)
	}
	if _, err = s.RecordApprovalFacts(ctx, ApprovalRunFacts{Run: "", Author: "x"}, exactCheck); !codeIs(err, "E607") {
		t.Fatalf("empty run: %v", err)
	}
	if _, err = s.RecordApprovalFacts(ctx, ApprovalRunFacts{Run: "r2"}, nil); !codeIs(err, "E607") {
		t.Fatalf("nil check: %v", err)
	}
}

func TestTrustApprovalsPending(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for _, r := range []string{"a", "b", "c"} {
		approvalSeed(t, s, r)
	}
	// d needs no approval; e is unrecorded and trusted by source_trust.
	if err := s.CreatePipeline(ctx, PipelineRow{ID: "d", Revision: "x", Project: "/p", Trigger: "push", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "c", SelectionMode: "full", Status: "queued"}); err != nil {
		t.Fatal(err)
	}
	needs := func(st ApprovalState) bool { return st.Requires || (!st.Facts && st.SourceTrust == "untrusted") }
	if _, err := s.ApproveRun(ctx, ApproveInput{Run: "b", Approver: "bob"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.PendingApprovals(ctx, needs, exactCheck)
	if err != nil {
		t.Fatal(err)
	}
	var runs []string
	for _, g := range got {
		runs = append(runs, g.Run)
	}
	if !reflect.DeepEqual(runs, []string{"a", "c"}) {
		t.Fatalf("pending %v: b is approved, d needs none", runs)
	}
	moveRun(t, s, "b", "sha-9", "cfg-1")
	got, _ = s.PendingApprovals(ctx, needs, exactCheck)
	if len(got) != 3 {
		t.Fatalf("a moved head puts b back on the list: %d", len(got))
	}
	if _, err = s.writer.Exec("UPDATE pipeline SET finished_at=1,status='passed' WHERE id='a'"); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.PendingApprovals(ctx, needs, exactCheck); len(got) != 2 || strings.Contains(got[0].Run+got[1].Run, "a") {
		t.Fatalf("a finished run is not pending: %v", got)
	}
}
