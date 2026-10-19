package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// admitSeed queues attempt id of job name in pipeline pipe of project proj.
func admitSeed(t *testing.T, s *Store, pipe, proj, name, id string) {
	t.Helper()
	ctx := context.Background()
	var have int
	if err := s.readers.QueryRowContext(ctx, "SELECT count(*) FROM pipeline WHERE id=?", pipe).Scan(&have); err != nil {
		t.Fatal(err)
	}
	if have == 0 {
		if err := s.CreatePipeline(ctx, PipelineRow{ID: pipe, Revision: "rev", Project: proj, Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CreateJob(ctx, JobRow{ID: "job-" + id, PipelineID: pipe, Name: name, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAttempt(ctx, AttemptRow{ID: id, JobID: "job-" + id, N: 1}, ""); err != nil {
		t.Fatal(err)
	}
}

func admitDemand(id, runner string) Demand {
	return Demand{AttemptID: id, RunnerID: runner, LeaseID: "lease-" + id, TokenDigest: "digest", CPU: 1, MemMB: 1, CapacityCPU: 100, CapacityMemMB: 100, TTL: time.Minute}
}

func TestSchedAdmissionCounters(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	admitSeed(t, s, "pipe", "proj", "build", "a1")
	admitSeed(t, s, "pipe", "proj", "build", "a2")
	admitSeed(t, s, "pipe", "proj", "lint", "a3")
	admitSeed(t, s, "other", "proj2", "build", "b1")
	req := AdmitRequest{User: "ann", Team: "core", Provider: "prov"}
	var seen []AdmitCounts
	record := func(_ context.Context, v AdmitView) error { seen = append(seen, v.Counts); return nil }
	for _, id := range []string{"a1", "a2", "a3"} {
		if _, err := s.AdmitPlacement(ctx, "host", admitDemand(id, "r1"), req, record); err != nil {
			t.Fatal(err)
		}
	}
	other := AdmitRequest{User: "bob", Team: "web", Provider: "prov2"}
	if _, err := s.AdmitPlacement(ctx, "host", admitDemand("b1", "r2"), other, record); err != nil {
		t.Fatal(err)
	}
	want := []AdmitCounts{
		{},
		{Pipeline: 1, Job: 1, Runner: 1, Provider: 1, Project: 1, User: 1, Team: 1, Global: 1},
		{Pipeline: 2, Job: 0, Runner: 2, Provider: 2, Project: 2, User: 2, Team: 2, Global: 2},
		{Pipeline: 0, Job: 0, Runner: 0, Provider: 0, Project: 0, User: 0, Team: 0, Global: 3},
	}
	for i, w := range want {
		if seen[i] != w {
			t.Fatalf("admission %d counters = %+v, want %+v", i, seen[i], w)
		}
	}
}

func TestSchedAdmissionAtomic(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	admitSeed(t, s, "pipe", "proj", "build", "a1")
	refuse := errors.New("limit.global")
	if _, err := s.AdmitPlacement(ctx, "host", admitDemand("a1", "r1"), AdmitRequest{}, func(context.Context, AdmitView) error { return refuse }); !errors.Is(err, refuse) {
		t.Fatalf("step error not returned: %v", err)
	}
	state, _, err := s.AttemptStateEpoch(ctx, "a1")
	if err != nil || state != "queued" {
		t.Fatalf("refused admission changed the attempt: %s %v", state, err)
	}
	snap, err := s.WorldSnapshot(ctx)
	if err != nil || len(snap.Active) != 0 || len(snap.Reservations) != 0 {
		t.Fatalf("refused admission left rows: %+v %v", snap, err)
	}
	if err = s.RecordRefusal(ctx, "a1", "limit.global", "1 of 1"); err != nil {
		t.Fatal(err)
	}
	if r, d, ok, e := s.Refusal(ctx, "a1"); e != nil || !ok || r != "limit.global" || d != "1 of 1" {
		t.Fatalf("refusal: %q %q %v %v", r, d, ok, e)
	}
	if _, err = s.AdmitPlacement(ctx, "host", admitDemand("a1", "r1"), AdmitRequest{User: "u", Provider: "p", WorldDigest: "dg"}); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, _ := s.Refusal(ctx, "a1"); ok {
		t.Fatal("admission kept the refusal")
	}
	if u, _, p, r, dg, ok, e := s.AdmittedIdentity(ctx, "a1"); e != nil || !ok || u != "u" || p != "p" || r != "r1" || dg != "dg" {
		t.Fatalf("identity: %q %q %q %q %v %v", u, p, r, dg, ok, e)
	}
	if _, err = s.AdmitPlacement(ctx, "host", admitDemand("a1", "r1"), AdmitRequest{}); !codeIs(err, "E605") {
		t.Fatalf("second admission of a leased attempt: %v", err)
	}
}

func TestSchedAdmissionCapacityAndFence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	admitSeed(t, s, "pipe", "proj", "x", "a1")
	admitSeed(t, s, "pipe", "proj", "y", "a2")
	d := admitDemand("a1", "r1")
	d.CapacityCPU = 1
	fence, err := s.AdmitPlacement(ctx, "host", d, AdmitRequest{Incarnation: 3})
	if err != nil || fence != LeaseEpoch(3, 0) {
		t.Fatalf("remote fence = %d, %v", fence, err)
	}
	l, err := s.LeaseGet(ctx, "lease-a1")
	if err != nil || l.Epoch != fence {
		t.Fatalf("lease epoch = %+v %v", l, err)
	}
	d2 := admitDemand("a2", "r1")
	d2.CapacityCPU = 1
	if _, err = s.AdmitPlacement(ctx, "host", d2, AdmitRequest{}); !codeIs(err, "E604") {
		t.Fatalf("host reservation full: %v", err)
	}
	if _, err = s.AdmitPlacement(ctx, "other-host", d2, AdmitRequest{}); err != nil {
		t.Fatalf("a different host has its own reservation: %v", err)
	}
	if _, err = s.AdmitPlacement(ctx, "host", Demand{}, AdmitRequest{}); !codeIs(err, "E604") {
		t.Fatalf("invalid demand: %v", err)
	}
	if _, err = s.AdmitPlacement(ctx, "host", admitDemand("missing", "r"), AdmitRequest{}); !codeIs(err, "E604") {
		t.Fatalf("unknown attempt: %v", err)
	}
}
