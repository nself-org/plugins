package exec

import (
	"testing"
	"time"
)

// A run starts several jobs within a moment: only the first container sweep of
// a coordinator in a TTL window runs (D-1008, two docker spawns per job start).
func TestSweepDueClaimsOncePerWindow(t *testing.T) {
	t0 := time.Unix(1_800_000_000, 0)
	const id = "sweep-due-ci1008"
	if !sweepDue(id, t0) {
		t.Fatal("the first caller must sweep")
	}
	if sweepDue(id, t0.Add(time.Second)) {
		t.Fatal("a second caller inside the window must not sweep")
	}
	if !sweepDue("other-"+id, t0.Add(time.Second)) {
		t.Fatal("another coordinator has its own window")
	}
	if !sweepDue(id, t0.Add(containerSweepTTL)) {
		t.Fatal("the window must reopen after the TTL")
	}
}

// A sweep that failed did not clean up what a crashed job left: its claim is
// given back so the next job start sweeps again, but a release never undoes a
// newer window's claim.
func TestSweepReleaseReopensOnlyTheFailedWindow(t *testing.T) {
	t0 := time.Unix(1_800_000_000, 0)
	const id = "sweep-release-ci1008"
	if !sweepDue(id, t0) {
		t.Fatal("the first caller must sweep")
	}
	sweepRelease(id, t0)
	if !sweepDue(id, t0.Add(time.Second)) {
		t.Fatal("after a failed sweep the next caller must sweep again")
	}
	sweepRelease(id, t0) // stale release: the claim at t0+1s is newer
	if sweepDue(id, t0.Add(2*time.Second)) {
		t.Fatal("a release of an older claim must not reopen the newer window")
	}
}
