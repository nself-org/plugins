package storetest

// Purpose: fixtures of other packages' tests lease an attempt through admission.
// Store.Transition refuses queued -> leased (R-SEC-22), so a test that needs a
// leased attempt calls Lease, which admits it with AdmitPlacement: the lease row,
// the host reservation and the counters are written, as in production.
// Inputs:  a store and a queued attempt id.
// Outputs: the attempt is leased; its epoch moves up by one, as a Transition would.
// Constraints: test fixtures only. The module's non-test code must not import
// this package (TestAdmitChokepoint scans for it); it is the one test-support
// function the chokepoint scan allows to name AdmitPlacement.

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Admit admits queued attempt id at its current epoch on a host with room for
// any number of fixtures. It returns an error so it can sit in a list of setup
// steps; Lease is the form that fails the test.
func Admit(ctx context.Context, s *store.Store, id string) error {
	_, epoch, err := s.AttemptStateEpoch(ctx, id)
	if err != nil {
		return err
	}
	d := store.Demand{AttemptID: id, RunnerID: "storetest", LeaseID: "lease-" + id, TokenDigest: "digest", CPU: 1, MemMB: 1, CapacityCPU: 1 << 20, CapacityMemMB: 1 << 20, TTL: time.Hour, Epoch: epoch}
	_, err = s.AdmitPlacement(ctx, "storetest-host", d, store.AdmitRequest{})
	return err
}

// Lease is Admit that fails the test when admission is refused.
func Lease(t testing.TB, s *store.Store, id string) {
	t.Helper()
	if err := Admit(context.Background(), s, id); err != nil {
		t.Fatalf("lease %s: %v", id, err)
	}
}
