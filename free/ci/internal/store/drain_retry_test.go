package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestDrainHandBackPreservesInfraBudget(t *testing.T) {
	for _, maximum := range []int{0, 1} {
		t.Run(fmt.Sprint(maximum), func(t *testing.T) {
			ctx := context.Background()
			s := testStore(t)
			now := time.Unix(100, 0)
			s.now = func() time.Time { return now }
			seed(t, s, "drain")
			if _, err := s.writer.ExecContext(ctx, "UPDATE job SET infra_max=? WHERE id='jdrain'", maximum); err != nil {
				t.Fatal(err)
			}
			id := "drain"
			for n, reason := range []string{DrainHandBack, DrainHandBack, "lease.expired", "lease.expired"} {
				d := Demand{AttemptID: id, LeaseID: fmt.Sprint("l", n), RunnerID: "runner", CPU: 1, MemMB: 1, CapacityCPU: 10, CapacityMemMB: 10, TTL: time.Second}
				if _, err := s.LeaseGrant(ctx, "host", d, 1); err != nil {
					t.Fatal(err)
				}
				now = now.Add(2 * time.Second)
				l, err := s.LeaseGet(ctx, d.LeaseID)
				if err != nil {
					t.Fatal(err)
				}
				if err = s.LeaseFail(ctx, l, now, reason, true); err != nil {
					t.Fatal(err)
				}
				state, _, err := s.AttemptStateEpoch(ctx, id)
				want := "retryable"
				if n >= maximum+2 {
					want = "needs-operator"
				}
				if err != nil || string(state) != want {
					t.Fatalf("n=%d reason=%s state=%s want=%s err=%v", n, reason, state, want, err)
				}
				if want == "needs-operator" {
					break
				}
				id = fmt.Sprintf("%s-retry-%d", id, n+2)
			}
		})
	}
}
