package store

// Purpose: D-1332 (P7-TRUST-08): the placement snapshot carries the trust class and the secret flag of
// each active attempt, so the world builder can tell what runs on a runner.

import (
	"context"
	"testing"
)

func seedClass(t *testing.T, s *Store, id, sourceTrust string, secretBearing bool) {
	t.Helper()
	ctx := context.Background()
	p := PipelineRow{ID: "p" + id, Revision: "rev", Project: "proj", Trigger: "local", SourceTrust: sourceTrust, PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	if err := s.CreatePipeline(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateJob(ctx, JobRow{ID: "j" + id, PipelineID: p.ID, Name: id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i", SecretBearing: secretBearing}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAttempt(ctx, AttemptRow{ID: id, JobID: "j" + id, N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	leaseOn(t, s, id, "node-"+id)
}

func TestWorldSnapshotCarriesTrustClassAndSecretFlag(t *testing.T) {
	s := testStore(t)
	seedClass(t, s, "own", "owner", false)
	seedClass(t, s, "fork", "untrusted", false)
	seedClass(t, s, "sec", "internal", true)
	snap, err := s.WorldSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]SchedAttempt{}
	for _, a := range snap.Active {
		got[a.RunnerID] = a
	}
	for runner, want := range map[string]struct {
		trust  string
		secret bool
	}{"node-own": {"owner", false}, "node-fork": {"untrusted", false}, "node-sec": {"internal", true}} {
		a, ok := got[runner]
		if !ok || a.Trust != want.trust || a.SecretBearing != want.secret {
			t.Fatalf("%s: got %+v, want trust %q secret %v", runner, a, want.trust, want.secret)
		}
	}
}
