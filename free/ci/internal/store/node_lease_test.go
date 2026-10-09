package store

import (
	"context"
	"testing"
	"time"
)

func TestNodeLeaseStore(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(100, 0)
	s := testStore(t)
	s.now = func() time.Time { return now }
	seed(t, s, "lease-attempt")
	d := Demand{AttemptID: "lease-attempt", LeaseID: "lease-1", RunnerID: "runner-1", TokenDigest: "token", CPU: 1, MemMB: 1, CapacityCPU: 2, CapacityMemMB: 2, TTL: 45 * time.Second}
	epoch, err := s.LeaseGrant(ctx, "host", d, 1)
	if err != nil || epoch != 1<<32|1 {
		t.Fatalf("grant epoch %d: %v", epoch, err)
	}
	if _, err := s.LeaseGrant(ctx, "host", d, 1); !codeIs(err, "E605") {
		t.Fatalf("duplicate grant: %v", err)
	}
	if err := s.Transition(ctx, d.AttemptID, "leased", "running", TransitionDetail{Epoch: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.LeaseRenew(ctx, d.LeaseID, epoch-1, now, d.TTL, true); !codeIs(err, "E662") {
		t.Fatalf("stale renewal: %v", err)
	}
	if err := s.LeaseFence(ctx, d.LeaseID, epoch-1); !codeIs(err, "E662") {
		t.Fatalf("stale carrier log: %v", err)
	}
	if err := s.LeaseResult(ctx, d.LeaseID, epoch-1); !codeIs(err, "E662") {
		t.Fatalf("stale result: %v", err)
	}
	now = now.Add(20 * time.Second)
	if err := s.LeaseRenew(ctx, d.LeaseID, epoch, now, d.TTL, true); err != nil {
		t.Fatal(err)
	}
	expired, err := s.LeaseExpired(ctx, now.Add(44*time.Second))
	if err != nil || len(expired) != 0 {
		t.Fatalf("early expiry: %v %+v", err, expired)
	}
	expired, err = s.LeaseExpired(ctx, now.Add(45*time.Second))
	if err != nil || len(expired) != 1 {
		t.Fatalf("missing expiry: %v %+v", err, expired)
	}
	if err := s.LeaseFail(ctx, expired[0], now, "lease.expired", true); !codeIs(err, "E605") {
		t.Fatalf("expired lease before clock: %v", err)
	}
	now = now.Add(45 * time.Second)
	if err := s.LeaseFail(ctx, expired[0], now, "lease.expired", true); err != nil {
		t.Fatal(err)
	}
	runner, err := s.LeasePreviousRunner(ctx, d.AttemptID)
	if err != nil || runner != d.RunnerID {
		t.Fatalf("previous runner %q: %v", runner, err)
	}
	d.AttemptID, d.LeaseID, d.Epoch = "lease-attempt-retry-2", "lease-2", 0
	next, err := s.LeaseGrant(ctx, "host", d, 2)
	if err != nil || next <= epoch {
		t.Fatalf("epoch reused after retry %d <= %d: %v", next, epoch, err)
	}
}

func TestGraceSkipsFinalizing(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(100, 0)
	s := testStore(t)
	s.now = func() time.Time { return now }
	seed(t, s, "finalizing-attempt")
	epoch, err := s.LeaseGrant(ctx, "host", Demand{AttemptID: "finalizing-attempt", LeaseID: "finalizing-lease", RunnerID: "runner", CPU: 1, MemMB: 1, CapacityCPU: 2, CapacityMemMB: 2, TTL: 45 * time.Second}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, "finalizing-attempt", "leased", "running", TransitionDetail{Epoch: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.LeaseResult(ctx, "finalizing-lease", epoch); err != nil {
		t.Fatal(err)
	}
	before, err := s.LeaseGet(ctx, "finalizing-lease")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(20 * time.Second)
	changed, err := s.LeaseGrace(ctx, "new", 2, now, 45*time.Second)
	if err != nil || changed != 0 {
		t.Fatalf("finalizing lease extended: %d %v", changed, err)
	}
	after, err := s.LeaseGet(ctx, "finalizing-lease")
	if err != nil || after.ExpiresAt != before.ExpiresAt {
		t.Fatalf("deadline changed: %d to %d: %v", before.ExpiresAt, after.ExpiresAt, err)
	}
}

func TestNodeLeaseGraceAndBreaker(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(100, 0)
	s := testStore(t)
	s.now = func() time.Time { return now }
	seed(t, s, "a")
	d := Demand{AttemptID: "a", LeaseID: "l", RunnerID: "r", CPU: 1, MemMB: 1, CapacityCPU: 2, CapacityMemMB: 2, TTL: 45 * time.Second}
	if _, err := s.LeaseGrant(ctx, "h", d, 1); err != nil {
		t.Fatal(err)
	}
	now = now.Add(300 * time.Second)
	count, err := s.LeaseGrace(ctx, "new-coordinator", 2, now, 45*time.Second)
	if err != nil || count != 1 {
		t.Fatalf("grace: %d %v", count, err)
	}
	if _, err := s.LeaseGrace(ctx, "new-coordinator", 2, now, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	l, err := s.LeaseGet(ctx, "l")
	if err != nil || l.ExpiresAt < now.Add(45*time.Second).UnixNano() {
		t.Fatalf("grace shortened lease: %+v %v", l, err)
	}
	rows, err := s.LeaseExpired(ctx, now)
	if err != nil || len(rows) != 0 {
		t.Fatalf("expired own downtime: %v %+v", err, rows)
	}
	for i := 1; i <= 4; i++ {
		n, trip, err := s.LeaseFailure(ctx, "node", true, 3)
		if err != nil || n != i || trip != (i >= 3) {
			t.Fatalf("breaker %d: %d %t %v", i, n, trip, err)
		}
	}
	n, trip, err := s.LeaseFailure(ctx, "node", false, 3)
	if err != nil || n != 4 || trip {
		t.Fatalf("code failure reset streak: %d %t %v", n, trip, err)
	}
}
