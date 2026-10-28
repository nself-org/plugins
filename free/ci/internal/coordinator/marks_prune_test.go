package coordinator

// Purpose: the maintenance prune of provider marks (D-1319).

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestPruneForgetsOldReconcileMarks(t *testing.T) {
	ctx, r := context.Background(), newRig(t, nil)
	now := r.clk.Now()
	day := 24 * time.Hour
	put := func(name, key string, age time.Duration) {
		t.Helper()
		if err := r.st.ProviderMarkPut(ctx, name, key, now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	put("github-reconcile", "run|o/r|1", 8*day)
	put("github-reconcile", "run|o/r|2", 6*day)
	put("github-reconcile", "demand|o/r|1", 8*day)
	put("github-reconcile", "term|o/r|abc", 29*day)
	put("github-reconcile", "enq|o/r|abc", 31*day)
	put("github-reconcile", "redeliver|o/r|g", 31*day)
	put("github-usage", "acct", 90*day) // another name: never touched by the github-reconcile rules
	r.c.prune(ctx, now)
	for _, m := range []struct {
		name, key string
		want      bool
	}{
		{"github-reconcile", "run|o/r|1", false}, {"github-reconcile", "run|o/r|2", true},
		{"github-reconcile", "demand|o/r|1", false}, {"github-reconcile", "term|o/r|abc", true},
		{"github-reconcile", "enq|o/r|abc", false}, {"github-reconcile", "redeliver|o/r|g", false},
		{"github-usage", "acct", true},
	} {
		if _, ok, err := r.st.ProviderMarkGet(ctx, m.name, m.key); err != nil || ok != m.want {
			t.Fatalf("mark %s/%s present = %v (%v), want %v", m.name, m.key, ok, err, m.want)
		}
	}
}

func TestPruneKeepsMarksWhenRetentionIsEmpty(t *testing.T) {
	ctx, r := context.Background(), newRig(t, func(c *Config) { c.MarkRetention = []MarkRetention{} })
	if err := r.st.ProviderMarkPut(ctx, "github-reconcile", "enq|o/r|a", r.clk.Now().Add(-90*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	r.c.prune(ctx, r.clk.Now())
	if _, ok, _ := r.st.ProviderMarkGet(ctx, "github-reconcile", "enq|o/r|a"); !ok {
		t.Fatal("an empty retention list must keep every mark")
	}
}

// The maintenance tick writes the void of a stale approval without anyone running `ci runs pending`
// (D-1342).
func TestMaintainSweepsStaleApprovals(t *testing.T) {
	ctx, path := context.Background(), filepath.Join(t.TempDir(), "state.db")
	r := newRig(t, func(c *Config) {
		st, err := store.Open(path, store.Options{Clock: c.Clock.Now})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = st.Close() })
		c.Store = st
	})
	p := store.PipelineRow{ID: "r1", Revision: "sha-1", Project: "/proj", Trigger: "push", SourceTrust: "untrusted", PrivacyZone: "local-only",
		PolicyDigest: "p", InputDigest: "cfg-1", SelectionMode: "full", Status: "queued"}
	if err := r.st.CreatePipeline(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := r.st.RecordApprovalFacts(ctx, store.ApprovalRunFacts{Run: "r1", Author: "mallory", HeadRepo: "fork/app", RequiresApproval: true}, approvalVoidCheck); err != nil {
		t.Fatal(err)
	}
	if _, err := r.st.ApproveRun(ctx, store.ApproveInput{Run: "r1", Approver: "bob", ExpectSHA: "sha-1", Check: approvalVoidCheck}); err != nil {
		t.Fatal(err)
	}
	mctx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { r.c.maintain(mctx); close(done) }()
	defer func() {
		stop()
		<-done
	}()
	waitFor(t, "the first beat to finish", func() bool { return r.clk.pending() >= 1 })
	if st, _, _ := r.st.ApprovalState(ctx, "r1"); st.Approval.VoidedAt != 0 {
		t.Fatal("an approval that still matches its run must stand")
	}
	// The pipeline moves to a new head, as a force-push would.
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err = db.Exec("UPDATE pipeline SET revision='sha-2' WHERE id='r1'"); err != nil {
		t.Fatal(err)
	}
	r.clk.Advance(r.c.cfg.Beat)
	waitFor(t, "the beat to void the stale approval", func() bool {
		st, _, _ := r.st.ApprovalState(ctx, "r1")
		return st.Approval.VoidedAt != 0
	})
	st, _, err := r.st.ApprovalState(ctx, "r1")
	if err != nil || !strings.Contains(st.Approval.VoidReason, "head") {
		t.Fatalf("the tick did not void the stale approval: %+v %v", st.Approval, err)
	}
}
