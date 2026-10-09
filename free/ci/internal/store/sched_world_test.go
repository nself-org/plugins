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
