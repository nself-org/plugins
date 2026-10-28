package store

import (
	"context"
	"testing"
	"time"
)

// doneRow leaves one finished github push run for key k, 71 h old.
func doneRow(t *testing.T) (*Store, string) {
	t.Helper()
	ctx := context.Background()
	s, clk := intakeStore(t)
	first := req("d1", "k")
	first.Event = "push"
	a, _ := s.IntakeEnqueue(ctx, first)
	if _, _, err := s.IntakeClaim(ctx, "me"); err != nil {
		t.Fatal(err)
	}
	if err := s.IntakeFinish(ctx, a.ID, "me", "run", "exit=0", true); err != nil {
		t.Fatal(err)
	}
	clk.add(71 * time.Hour)
	return s, a.ID
}

// A head-scan (Reconcile) trigger folds into a run of the same source and key that finished inside the
// redelivery horizon; a plain non-webhook trigger, another source or another key does not (D-1320).
func TestIntakeReconcileFoldsOnlySameSourceAndKey(t *testing.T) {
	for _, c := range []struct {
		name, source, key string
		reconcile         bool
	}{
		{"plain non-webhook trigger", "github", "k", false},
		{"another source", "gitlab", "k", true},
		{"another semantic key", "github", "other-key", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, _ := doneRow(t)
			r := req("s1", c.key)
			r.Webhook, r.Reconcile, r.Source, r.Event = false, c.reconcile, c.source, "pull_request"
			if b, err := s.IntakeEnqueue(context.Background(), r); err != nil || b.Kind != IntakeEnqueued {
				t.Fatalf("%+v, %v; want a fresh run", b, err)
			}
		})
	}
}

func TestIntakeReconcileFoldsWhatTheWebhookBuilt(t *testing.T) {
	ctx := context.Background()
	s, clk := intakeStore(t)
	first := req("d1", "k")
	first.Event = "push"
	a, _ := s.IntakeEnqueue(ctx, first)
	_, _, _ = s.IntakeClaim(ctx, "me")
	_ = s.IntakeFinish(ctx, a.ID, "me", "run", "exit=0", true)
	clk.add(71 * time.Hour)
	r := req("reconcile:o/r:abc1234", "k")
	r.Webhook, r.Reconcile, r.Event = false, true, "pull_request"
	if b, err := s.IntakeEnqueue(ctx, r); err != nil || b.Kind != IntakeCoalesced || b.ID != a.ID {
		t.Fatalf("reconcile 71 h after the run = %+v, %v; want coalesced into %s", b, err, a.ID)
	}
	if b, _ := s.IntakeEnqueue(ctx, r); b.Kind != IntakeDuplicate || b.ID != a.ID {
		t.Fatalf("the same sweep delivery again = %+v, want duplicate of %s", b, a.ID)
	}
	clk.add(2 * time.Hour)
	r.DeliveryID = "reconcile:o/r:abc1234:later"
	if b, _ := s.IntakeEnqueue(ctx, r); b.Kind != IntakeEnqueued {
		t.Fatalf("reconcile past the horizon = %+v, want a fresh run", b)
	}
}

// A run that failed to finish has no result to send again: the sweep builds the head.
func TestIntakeReconcileDoesNotFoldIntoFailedRow(t *testing.T) {
	ctx := context.Background()
	s, _ := intakeStore(t)
	a, _ := s.IntakeEnqueue(ctx, req("d1", "k"))
	_, _, _ = s.IntakeClaim(ctx, "me")
	_ = s.IntakeFinish(ctx, a.ID, "me", "", "error: boom", false)
	r := req("s1", "k")
	r.Webhook, r.Reconcile = false, true
	if b, _ := s.IntakeEnqueue(ctx, r); b.Kind != IntakeEnqueued {
		t.Fatalf("reconcile after a failed row = %+v, want a fresh run", b)
	}
}
