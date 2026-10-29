package coordinator

// Purpose: a head-scan delivery that folds into a finished run sends the stored result again (D-1320).

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// recordBoundRun leaves a pipeline row and the bound, passing evidence of a whole-gate run of testSHA.
func recordBoundRun(t *testing.T, st *store.Store, id string) {
	t.Helper()
	ctx := context.Background()
	if err := st.CreatePipeline(ctx, store.PipelineRow{ID: id, Revision: testSHA, Project: "/proj", Trigger: "push", SourceTrust: "trusted", PrivacyZone: "local-only",
		PolicyDigest: "p", InputDigest: "cfg", SelectionMode: "full", Status: "passed"}); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"revision":"` + testSHA + `","binding":{"state":"bound"},"result":"pass"}`)
	if err := st.RecordRunEvidence(ctx, "ev-"+id, id, testSHA, "cfg", body); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileDeliveryRereportsFinishedRun(t *testing.T) {
	ctx := context.Background()
	var got []store.IntakeRow
	r := newRig(t, func(c *Config) {
		c.Rereport = func(_ context.Context, row store.IntakeRow) error { got = append(got, row); return nil }
	})
	r.post("/webhook", "push", "g1", pushBody(testSHA))
	row, ok, err := r.st.IntakeClaim(ctx, "w")
	if err != nil || !ok {
		t.Fatal(err)
	}
	sweep := Delivery{Source: row.Source, Project: "o/r", Revision: testSHA, DeliveryID: "reconcile:o/r:" + testSHA, Reconcile: true}
	// The webhook's row is still queued: it reports for itself, nothing is sent again.
	if out, err := r.c.Enqueue(ctx, sweep); err != nil || out.Kind != store.IntakeCoalesced || len(got) != 0 {
		t.Fatalf("while the run is live: %+v %v, %d re-reports", out, err, len(got))
	}
	recordBoundRun(t, r.st, "run-1")
	if err = r.st.IntakeFinish(ctx, row.ID, "w", "run-1", "exit=0", true); err != nil {
		t.Fatal(err)
	}
	r.clk.Advance(2 * time.Hour)
	sweep.DeliveryID = "reconcile:o/r:" + testSHA + ":2"
	if out, err := r.c.Enqueue(ctx, sweep); err != nil || out.Kind != store.IntakeCoalesced || out.ID != row.ID {
		t.Fatalf("after the run: %+v %v, want coalesced into %s (no second build)", out, err, row.ID)
	}
	if len(got) != 1 || got[0].ID != row.ID || got[0].PipelineID != "run-1" {
		t.Fatalf("re-reports = %+v, want the finished row once", got)
	}
	if out, _ := r.c.Enqueue(ctx, sweep); out.Kind != store.IntakeDuplicate || len(got) != 2 {
		t.Fatalf("the same sweep delivery again: %+v, %d re-reports; want a duplicate that sends the result again", out, len(got))
	}
	if st, _ := r.st.IntakeStats(ctx); st.Queued != 0 {
		t.Fatalf("a second build was queued: %+v", st)
	}
}

func TestPlainDeliveryNeverRereports(t *testing.T) {
	ctx := context.Background()
	called := false
	r := newRig(t, func(c *Config) {
		c.Rereport = func(context.Context, store.IntakeRow) error { called = true; return nil }
	})
	r.post("/webhook", "push", "g1", pushBody(testSHA))
	row, _, _ := r.st.IntakeClaim(ctx, "w")
	_ = r.st.IntakeFinish(ctx, row.ID, "w", "run-1", "exit=0", true)
	if out, err := r.c.Enqueue(ctx, Delivery{Source: row.Source, Project: "o/r", Revision: testSHA, DeliveryID: "other"}); err != nil || out.Kind != store.IntakeEnqueued || called {
		t.Fatalf("a plain delivery: %+v %v rereported=%v", out, err, called)
	}
}

// A finished run with no result to send again (cancelled here) is not folded into: the sweep builds the head.
func TestReconcileDeliveryBuildsWhenFinishedRunHasNothingToResend(t *testing.T) {
	ctx := context.Background()
	called := false
	r := newRig(t, func(c *Config) {
		c.Rereport = func(context.Context, store.IntakeRow) error { called = true; return nil }
	})
	r.post("/webhook", "push", "g1", pushBody(testSHA))
	row, _, _ := r.st.IntakeClaim(ctx, "w")
	_ = r.st.IntakeFinish(ctx, row.ID, "w", "run-1", "exit=0", true) // no evidence recorded
	r.clk.Advance(2 * time.Hour)
	sweep := Delivery{Source: row.Source, Project: "o/r", Revision: testSHA, DeliveryID: "reconcile:o/r:" + testSHA, Reconcile: true}
	if out, err := r.c.Enqueue(ctx, sweep); err != nil || out.Kind != store.IntakeEnqueued || out.ID == row.ID || called {
		t.Fatalf("sweep after an unreportable run: %+v %v rereported=%v; want a new run", out, err, called)
	}
}
