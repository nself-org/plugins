//go:build scenarios && planted

package scenarios

// Purpose: planted regressions for scenario 10 (P7-SCHED-15, Epic D20). Each test injects one defect
// in test code only (a wrapped placer, a lying world, a lease written around Admit) and requires
// the SAME check function the real scenario uses to report it.
// Inputs:  the check functions of scenario10_test.go and the rig of s10_rig_test.go.
// Outputs: a t.Fatal when a planted defect goes unnoticed.
// Constraints: build tags `scenarios && planted`; production code is never edited.

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// TestPlantedPriorityBypassesEligibility: a placer that forces every high-priority job onto the
// first runner, eligible or not, must be caught.
func TestPlantedPriorityBypassesEligibility(t *testing.T) {
	w, jobs := s10PriorityWorld()
	bypass := func(w sched.World, jobs []sched.Job) sched.Result {
		res := sched.Place(w, jobs)
		placed := map[string]bool{}
		for _, p := range res.Placements {
			placed[p.JobKey] = true
		}
		for _, j := range jobs {
			if j.Priority == "high" && !placed[j.Key] {
				res.Placements = append(res.Placements, sched.Placement{JobKey: j.Key, RunnerID: w.Runners[0].ID, CPU: j.Requirements.CPU, MemMB: j.Requirements.MemMB})
			}
		}
		return res
	}
	if v := s10CheckPriority(bypass, w, jobs); len(v) == 0 {
		t.Fatal("planted regression not detected: a high-priority job was forced onto an ineligible runner and the check stayed silent")
	}
}

// TestPlantedReservationOvercommit: a world that claims 6 slots for a runner that has 2 lets the
// dispatcher overcommit it; the check against true capacity must report the breach.
func TestPlantedReservationOvercommit(t *testing.T) {
	lying := []sched.Runner{s10Runner("lan1", "lan", 6)}
	rig := newS10Rig(t, lying, nil, nil)
	rig.trueSlots = map[string]int{"lan1": 2} // what the runner really has
	for i := 0; i < 6; i++ {
		rig.submit("alpha", "")
	}
	rig.waitQueued(6)
	rig.release()
	rig.waitAdmitted(6)
	if v := s10CheckCapacity(rig.ctx, rig.st, rig.trueSlots, 8); len(v) == 0 {
		t.Fatal("planted regression not detected: 6 leases on a 2-slot runner and the capacity check stayed silent")
	}
	if len(rig.sink.violations()) == 0 {
		t.Fatal("planted regression not detected at admission time: the per-admission observer saw no breach")
	}
}

// TestPlantedLeaseOutsideAdmit: an attempt leased straight through the store, around
// dispatch.Admit, has no admission record and must be reported.
func TestPlantedLeaseOutsideAdmit(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	honest := queueAttempt(t, st, "pipe-alpha", "alpha", "honest")
	via := newDispatcher(t, dispatch.Config{Store: st, Poll: time.Millisecond, World: liveWorld(t, st, nil, s10Runner("local", "local", 2))})
	if _, err := via.Submit(ctx, dispatch.Job{AttemptID: honest, Sched: func() sched.Job { j := job(honest, "linux/amd64"); j.Project = "alpha"; return j }()}); err != nil {
		t.Fatal(err)
	}
	if v := s10CheckLeasesAdmitted(ctx, st, []string{honest}); len(v) != 0 {
		t.Fatalf("a lease granted by Admit was reported: %v", v)
	}
	rogue := queueAttempt(t, st, "pipe-alpha", "alpha", "rogue")
	_, epoch, err := st.AttemptStateEpoch(ctx, rogue)
	if err != nil {
		t.Fatal(err)
	}
	err = st.AdmitAndLease(ctx, "host", store.Demand{AttemptID: rogue, RunnerID: "local", LeaseID: "lease-" + rogue, TokenDigest: "digest", CPU: 1, MemMB: 128, CapacityCPU: 8, CapacityMemMB: 8192, Epoch: epoch})
	if err != nil {
		t.Fatal(err)
	}
	if v := s10CheckLeasesAdmitted(ctx, st, []string{honest, rogue}); len(v) == 0 {
		t.Fatal("planted regression not detected: an attempt leased around dispatch.Admit has no admission record and the check stayed silent")
	}
}
