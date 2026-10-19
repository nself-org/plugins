package store

import (
	"context"
	"testing"
	"time"
)

func TestSchedWorldQueries(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seed(t, s, "world")
	before, err := s.WorldSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if before.Queued["proj"] != 1 || before.Projects["pworld"] != "proj" {
		t.Fatalf("queued/project: %+v", before)
	}
	if err = s.AdmitAndLease(ctx, "host", Demand{AttemptID: "world", RunnerID: "runner", LeaseID: "lease-world", CPU: 2, MemMB: 64, CapacityCPU: 4, CapacityMemMB: 128, TTL: time.Minute}); err != nil {
		t.Fatal(err)
	}
	after, err := s.WorldSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Active) != 1 || after.Active[0].RunnerID != "runner" || after.Queued["proj"] != 0 || after.Reservations["runner"].CPU != 2 {
		t.Fatalf("active snapshot: %+v", after)
	}
	if _, err = s.writer.ExecContext(ctx, "UPDATE attempt SET previous_runner_id='earlier' WHERE id='world'"); err != nil {
		t.Fatal(err)
	}
	retried, err := s.WorldSnapshot(ctx)
	if err != nil || retried.Previous["world"] != "earlier" || retried.PreviousByJob["jworld"] != "earlier" {
		t.Fatalf("previous: %+v %v", retried, err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.WorldSnapshot(ctx); err == nil {
		t.Fatal("closed store did not surface a read error")
	}
}

// TestSchedWorldQueuedByBand (D-1265): a queued attempt carries its band, and the
// snapshot counts the interactive ones apart. An attempt with no recorded band is
// counted as interactive (unknown is never assumed to be idle work); a recorded
// normal or background band is not.
func TestSchedWorldQueuedByBand(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seed(t, s, "bg")
	if err := s.SetAttemptBand(ctx, "bg", "background"); err != nil {
		t.Fatal(err)
	}
	snap, err := s.WorldSnapshot(ctx)
	if err != nil || snap.Queued["proj"] != 1 || snap.QueuedInteractive["proj"] != 0 {
		t.Fatalf("a lone background attempt is queued but not interactive: %+v %v", snap, err)
	}
	seed(t, s, "norm")
	if err = s.SetAttemptBand(ctx, "norm", "normal"); err != nil {
		t.Fatal(err)
	}
	seed(t, s, "unknown") // no band recorded
	snap, err = s.WorldSnapshot(ctx)
	if err != nil || snap.Queued["proj"] != 3 || snap.QueuedInteractive["proj"] != 1 {
		t.Fatalf("only the attempt without a band counts as interactive: %+v %v", snap, err)
	}
	if err = s.SetAttemptBand(ctx, "unknown", "interactive"); err != nil {
		t.Fatal(err)
	}
	if snap, err = s.WorldSnapshot(ctx); err != nil || snap.QueuedInteractive["proj"] != 1 {
		t.Fatalf("an interactive attempt counts: %+v %v", snap, err)
	}
	if err = s.AdmitAndLease(ctx, "host", Demand{AttemptID: "unknown", RunnerID: "runner", LeaseID: "lease-u", CPU: 1, MemMB: 64, CapacityCPU: 4, CapacityMemMB: 128, TTL: time.Minute}); err != nil {
		t.Fatal(err)
	}
	if snap, err = s.WorldSnapshot(ctx); err != nil || snap.QueuedInteractive["proj"] != 0 {
		t.Fatalf("a leased attempt is no longer queued: %+v %v", snap, err)
	}
	if err = s.SetAttemptBand(ctx, "unknown", "background"); err != nil {
		t.Fatal(err)
	}
	var band string
	if err = s.readers.QueryRowContext(ctx, "SELECT coalesce(band,'') FROM attempt WHERE id='unknown'").Scan(&band); err != nil || band != "interactive" {
		t.Fatalf("a band is recorded while the attempt is queued only: %q %v", band, err)
	}
}
