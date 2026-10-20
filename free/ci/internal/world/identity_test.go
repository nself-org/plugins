package world

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// identityFixture admits attempts under chosen identities. user/team "" means the lease
// is made through AdmitAndLease, which records no placement identity.
func identityFixture(t *testing.T, pipes map[string][2]string, admits []admit) Deps {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{Clock: func() time.Time { return time.Unix(100, 0) }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	for id, who := range pipes {
		row := store.PipelineRow{ID: id, Project: "project", Revision: "rev", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued", User: who[0], Team: who[1]}
		if err = s.CreatePipeline(ctx, row); err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range admits {
		if err = s.CreateJob(ctx, store.JobRow{ID: "job-" + a.id, PipelineID: a.pipe, Name: a.id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
			t.Fatal(err)
		}
		if err = s.CreateAttempt(ctx, store.AttemptRow{ID: a.id, JobID: "job-" + a.id, N: 1}, ""); err != nil {
			t.Fatal(err)
		}
		d := store.Demand{AttemptID: a.id, RunnerID: "r1", LeaseID: "lease-" + a.id, CPU: 1, MemMB: 1, CapacityCPU: 100, CapacityMemMB: 100}
		if a.user == "" {
			err = s.AdmitAndLease(ctx, "host", d)
		} else {
			_, err = s.AdmitPlacement(ctx, "host", d, store.AdmitRequest{User: a.user, Team: a.team, Provider: "local"})
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	return Deps{Store: s, Policy: func(context.Context, string) (sched.Policy, error) { return sched.Policy{}, nil }, Clock: func() time.Time { return time.Unix(100, 0) }}
}

type admit struct{ id, pipe, user, team string }

// TestWorldCountsUserAndTeamPerIdentity is D-1272: one identity must not be limited by
// another's load. An active attempt with no recorded identity still counts for everyone.
func TestWorldCountsUserAndTeamPerIdentity(t *testing.T) {
	d := identityFixture(t, map[string][2]string{"mine": {"alice", "t1"}, "other": {"bob", "t2"}, "legacy": {"", ""}}, []admit{
		{"a1", "mine", "alice", "t1"}, {"a2", "mine", "alice", "t1"}, {"a3", "other", "alice", "t2"},
		{"b1", "other", "bob", "t1"}, {"b2", "other", "bob", "t2"}, {"b3", "other", "bob", "t2"}, {"b4", "other", "bob", "t2"},
	})
	w, _, err := Build(context.Background(), d, sched.Pipeline{ID: "mine"})
	if err != nil {
		t.Fatal(err)
	}
	if w.Counts.Global != 7 || w.Counts.User != 3 || w.Counts.Team != 3 {
		t.Fatalf("alice/t1: counts = global %d user %d team %d, want 7 3 3", w.Counts.Global, w.Counts.User, w.Counts.Team)
	}
	w, _, err = Build(context.Background(), d, sched.Pipeline{ID: "other"})
	if err != nil {
		t.Fatal(err)
	}
	if w.Counts.User != 4 || w.Counts.Team != 4 {
		t.Fatalf("bob/t2: counts = user %d team %d, want 4 4", w.Counts.User, w.Counts.Team)
	}
	w, _, err = Build(context.Background(), d, sched.Pipeline{ID: "legacy"})
	if err != nil {
		t.Fatal(err)
	}
	if w.Counts.User != w.Counts.Global || w.Counts.Team != w.Counts.Global {
		t.Fatalf("a pipeline with no identity keeps the fail-closed global bound: %+v", w.Counts)
	}
}

func TestWorldCountsUnknownAttemptIdentityCountsForEveryone(t *testing.T) {
	d := identityFixture(t, map[string][2]string{"mine": {"alice", "t1"}}, []admit{
		{"a1", "mine", "alice", "t1"}, {"l1", "mine", "", ""}, {"b1", "mine", "bob", "t2"},
	})
	w, _, err := Build(context.Background(), d, sched.Pipeline{ID: "mine"})
	if err != nil {
		t.Fatal(err)
	}
	if w.Counts.Global != 3 || w.Counts.User != 2 || w.Counts.Team != 2 {
		t.Fatalf("counts = global %d user %d team %d, want 3 2 2", w.Counts.Global, w.Counts.User, w.Counts.Team)
	}
}
