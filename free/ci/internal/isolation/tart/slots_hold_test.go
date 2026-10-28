package tart

// Purpose: the slot hold the burst provider takes when it grants a vm slot (D-1381): a granted slot is busy in
//          the shared ledger before its job is wrapped, so a non-burst vm job cannot take it, the granted
//          job's own Wrap still succeeds, and a hold whose job never ran is given back by Teardown.
// Inputs:  the fake tart rig.
// Outputs: pass or fail.
// Constraints: no real Tart, no network.

import (
	"context"
	"strings"
	"testing"
)

func TestHoldTakesASlotAndTeardownGivesItBack(t *testing.T) {
	r := newRig(t, nil)
	ctx := context.Background()
	h1, h2, h3 := isolationAttemptFor("h1", "coord-test"), isolationAttemptFor("h2", "coord-test"), isolationAttemptFor("h3", "coord-test")
	if err := r.p.Hold(ctx, h1); err != nil {
		t.Fatal(err)
	}
	if err := r.p.Hold(ctx, h1); err != nil { // holding twice is one slot
		t.Fatal(err)
	}
	if err := r.p.Hold(ctx, h2); err != nil {
		t.Fatal(err)
	}
	if err := r.p.Hold(ctx, h3); err == nil || !strings.Contains(err.Error(), ReasonSlots) {
		t.Fatalf("a third hold must find both slots busy, got %v", err)
	}
	if err := r.p.Teardown(ctx, h1); err != nil {
		t.Fatal(err)
	}
	if err := r.p.Hold(ctx, h3); err != nil {
		t.Fatalf("Teardown of an attempt that never ran gives its hold back: %v", err)
	}
	if err := r.p.Teardown(ctx, isolationAttemptFor("never-held", "coord-test")); err != nil || r.p.slots.Free(0, 0) != 0 {
		t.Fatalf("Teardown of an attempt without a hold must not free a slot: %v, free %d", err, r.p.slots.Free(0, 0))
	}
}

// TestHeldSlotIsRefusedToANonBurstJobButNotToItsOwnJob: both slots are held by granted burst jobs; a non-burst
// vm job is refused at once, a granted job's own Wrap takes its hold over (one slot, never two) and frees it
// at the end, after which the non-burst job fits.
func TestHeldSlotIsRefusedToANonBurstJobButNotToItsOwnJob(t *testing.T) {
	r := newRig(t, nil)
	ctx := context.Background()
	for _, id := range []string{"b1", "b2"} {
		if err := r.p.Hold(ctx, isolationAttemptFor(id, "coord-test")); err != nil {
			t.Fatal(err)
		}
	}
	res, _ := r.run(ctx, r.spec("n1", "true"))
	if !res.Refused || res.Err == nil || !strings.Contains(res.Err.Error(), "E678") {
		t.Fatalf("a non-burst job must be refused while both slots are held, got %+v", res)
	}
	r.noClones()
	if res, _ = r.run(ctx, r.spec("b1", "true")); res.Err != nil || res.Refused {
		t.Fatalf("the granted job must run on its held slot, got %+v", res)
	}
	if free := r.p.slots.Free(0, 0); free != 1 {
		t.Fatalf("b1 ran and released: %d slots free, want 1 (b2 still holds one)", free)
	}
	if res, _ = r.run(ctx, r.spec("n2", "true")); res.Err != nil || res.Refused {
		t.Fatalf("the freed slot must fit a non-burst job, got %+v", res)
	}
	r.noClones()
}
