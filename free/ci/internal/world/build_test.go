package world

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func worldFixture(t *testing.T) (Deps, sched.Pipeline) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{Clock: func() time.Time { return time.Unix(100, 0) }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	var local model.Capability
	for i, name := range []string{"laptop", "lan-mac", "vps"} {
		raw, err := os.ReadFile("../model/testdata/capability/valid/" + name + ".valid.json")
		if err != nil {
			t.Fatal(err)
		}
		var c model.Capability
		if err = json.Unmarshal(raw, &c); err != nil {
			t.Fatal(err)
		}
		c.Identity.ID = fmt.Sprintf("01J%023d", i+1)
		if i == 0 {
			local = c
		}
		state := "online"
		if i == 0 {
			state = "revoked"
		}
		if i == 2 {
			source := "inventory"
			c.DeployHost.Matched = true
			c.DeployHost.Source = &source
		}
		doc, e := json.Marshal(c)
		if e != nil {
			t.Fatal(e)
		}
		err = s.NodePut(ctx, store.NodeRecord{ID: c.Identity.ID, Name: c.Identity.Name, State: state, Capability: doc}, "operator")
		if err != nil {
			t.Fatal(err)
		}
	}
	p := store.PipelineRow{ID: "pipe", Project: "project", Revision: "rev", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	if err = s.CreatePipeline(ctx, p); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		id := string(rune('a' + i))
		if err = s.CreateJob(ctx, store.JobRow{ID: "job" + id, PipelineID: p.ID, Name: id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
			t.Fatal(err)
		}
		if err = s.CreateAttempt(ctx, store.AttemptRow{ID: "attempt" + id, JobID: "job" + id, N: 1}, ""); err != nil {
			t.Fatal(err)
		}
		if err = s.AdmitAndLease(ctx, "host", store.Demand{AttemptID: "attempt" + id, RunnerID: "local", LeaseID: "lease" + id, CPU: 1, MemMB: 1, CapacityCPU: 10, CapacityMemMB: 10}); err != nil {
			t.Fatal(err)
		}
	}
	return Deps{Store: s, Local: &local, Policy: func(context.Context, string) (sched.Policy, error) { return sched.Policy{}, nil }, Clock: func() time.Time { return time.Unix(100, 0) }, LocalAddresses: func(context.Context) ([]string, error) { return []string{"127.0.0.1"}, nil }, ResolveHost: func(context.Context, string) ([]string, error) { return []string{"192.0.2.10"}, nil }}, sched.Pipeline{ID: "pipe"}
}

func TestWorldBuild(t *testing.T) {
	d, p := worldFixture(t)
	ctx := context.Background()
	w, digest, err := Build(ctx, d, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Runners) != 4 || w.Counts.Global != 5 || w.Counts.User != 5 || w.Counts.Team != 5 || w.Counts.Project != 5 {
		t.Fatalf("world: %+v", w.Counts)
	}
	local := w.Runners[len(w.Runners)-1]
	if local.ID != "local" || local.Avail.Slots != 0 {
		t.Fatalf("local capacity: %s %+v", local.ID, local.Avail)
	}
	if local.CoordinatorHost || local.CoordinatorMode != "" {
		t.Fatal("coordinator host inferred without provider")
	}
	const goldenDigest = "ca3343acf0baa8bd7275f9ab6973a5449507284a9b464948ba123765546f2a1a"
	if digest != goldenDigest {
		t.Fatalf("golden world digest: got %s want %s", digest, goldenDigest)
	}
	if w.Runners[0].Capability.Availability.State.Value == nil || *w.Runners[0].Capability.Availability.State.Value != "revoked" || !w.Runners[2].Decl.DeployHost.Matched {
		t.Fatal("golden lifecycle/deploy host facts missing")
	}
	again, other, err := Build(ctx, d, p)
	if err != nil || digest != other || !reflect.DeepEqual(w, again) {
		t.Fatalf("unstable build: %v %s %s", err, digest, other)
	}
	if err = d.Store.CreateJob(ctx, store.JobRow{ID: "queued-job", PipelineID: p.ID, Name: "queued", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
		t.Fatal(err)
	}
	if err = d.Store.CreateAttempt(ctx, store.AttemptRow{ID: "queued-attempt", JobID: "queued-job", N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	queued, _, err := Build(ctx, d, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range queued.Runners {
		if !queued.InteractiveQueued[r.ID] || r.QueuedAhead != 1 {
			t.Fatalf("queued work not applied to %s", r.ID)
		}
	}
	d.Local.Availability.Slots.Value = nil
	d.Local.Availability.Slots.Confidence = "unknown"
	unknown, _, err := Build(ctx, d, p)
	if err != nil || unknown.Runners[len(unknown.Runners)-1].Capability.Availability.Slots.Value != nil || unknown.Runners[len(unknown.Runners)-1].Avail.Slots != 0 {
		t.Fatalf("unknown capacity guessed: %v", err)
	}
	d.Policy = func(context.Context, string) (sched.Policy, error) { return sched.Policy{}, errors.New("policy fault") }
	partial, _, err := Build(ctx, d, p)
	if err == nil || len(partial.Runners) != 0 {
		t.Fatalf("partial world on failure: %+v %v", partial, err)
	}
	// A user or team limit at the global active count refuses even without identity facts.
	w.Policy.Limits.User = 5
	w.Policy.Limits.Team = 5
	placed := sched.Place(w, []sched.Job{{Key: "limit", Pipeline: sched.Pipeline{Support: []string{""}}}})
	userRefused, teamRefused := false, false
	for _, alt := range placed.Explanations[0].Alternatives {
		for _, reason := range alt.Reasons {
			if reason.Code == string(model.LimitUser) {
				userRefused = true
			}
			if reason.Code == string(model.LimitTeam) {
				teamRefused = true
			}
		}
	}
	if !userRefused || !teamRefused {
		t.Fatalf("global bound did not refuse user/team at N=5: %+v", placed)
	}
	d.Policy = func(context.Context, string) (sched.Policy, error) { return sched.Policy{}, nil }
	if err = d.Store.Close(); err != nil {
		t.Fatal(err)
	}
	partial, _, err = Build(ctx, d, p)
	if err == nil || !strings.Contains(err.Error(), "E699") || !strings.Contains(err.Error(), "E607") || len(partial.Runners) != 0 {
		t.Fatalf("store error: %+v %v", partial, err)
	}
}

func TestWorldContributors(t *testing.T) {
	contributions.Lock()
	contributions.byField = map[string]Contributor{}
	contributions.Unlock()
	hooks.Lock()
	hooks.labels = nil
	hooks.facts = nil
	hooks.Unlock()
	t.Cleanup(func() {
		contributions.Lock()
		contributions.byField = map[string]Contributor{}
		contributions.Unlock()
		hooks.Lock()
		hooks.labels = nil
		hooks.facts = nil
		hooks.Unlock()
	})
	if CoordinatorFactsRegistered() {
		t.Fatal("unexpected facts provider")
	}
	order := []string{}
	if err := Register(Contributor{Name: "providers", Field: "Hosted", Fn: func(_ context.Context, w *sched.World) error {
		order = append(order, "providers")
		w.Hosted = []sched.Runner{{ID: "hosted"}}
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if err := Register(Contributor{Name: "cache", Field: "Cache", Fn: func(_ context.Context, w *sched.World) error {
		order = append(order, "cache")
		w.Cache = map[string]map[string]sched.CacheFact{"job": {"local": {Confidence: "known"}}}
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"Hosted", "Cache"} {
		if err := Register(Contributor{Name: "duplicate", Field: field, Fn: func(context.Context, *sched.World) error { return nil }}); err == nil || !strings.Contains(err.Error(), "E699") {
			t.Fatalf("duplicate %s: %v", field, err)
		}
	}
	if err := RegisterLabelSource(func(_ context.Context, id string) ([]string, error) { return []string{"extra=" + id}, nil }); err != nil {
		t.Fatal(err)
	}
	if err := RegisterLabelSource(func(context.Context, string) ([]string, error) { return nil, nil }); err == nil {
		t.Fatal("second label source accepted")
	}
	if err := RegisterCoordinatorFacts(func(context.Context) (CoordinatorFacts, error) {
		return CoordinatorFacts{Serving: true, Mode: "personal"}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if !CoordinatorFactsRegistered() {
		t.Fatal("facts provider invisible")
	}
	if err := RegisterCoordinatorFacts(func(context.Context) (CoordinatorFacts, error) { return CoordinatorFacts{}, nil }); err == nil {
		t.Fatal("second facts provider accepted")
	}
	d, p := worldFixture(t)
	w, _, err := Build(context.Background(), d, p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"cache", "providers"}) || len(w.Hosted) != 1 || len(w.Cache) != 1 {
		t.Fatalf("contribution: %+v %v", w, order)
	}
	local := w.Runners[len(w.Runners)-1]
	if !local.CoordinatorHost || local.CoordinatorMode != "personal" || !strings.Contains(strings.Join(local.Capability.Identity.Labels, ","), "extra=local") {
		t.Fatalf("local hooks: %s %v %v", local.ID, local.CoordinatorHost, local.Capability.Identity.Labels)
	}
	if !strings.Contains(strings.Join(w.Runners[0].Capability.Identity.Labels, ","), "extra="+w.Runners[0].ID) {
		t.Fatal("node label source missing")
	}
	if err := Register(Contributor{Name: "identity", Field: "Counts", Fn: func(_ context.Context, w *sched.World) error { w.Counts.User = 1; w.Counts.Team = 2; return nil }}); err != nil {
		t.Fatal(err)
	}
	updated, _, err := Build(context.Background(), d, p)
	if err != nil || updated.Counts.User != 1 || updated.Counts.Team != 2 || updated.Counts.Global != 5 {
		t.Fatalf("identity count contribution: %+v %v", updated.Counts, err)
	}
	hooks.Lock()
	savedFacts := hooks.facts
	hooks.facts = func(context.Context) (CoordinatorFacts, error) {
		return CoordinatorFacts{}, errors.New("provider failed")
	}
	hooks.Unlock()
	failedFacts, _, err := Build(context.Background(), d, p)
	hooks.Lock()
	hooks.facts = savedFacts
	hooks.Unlock()
	if err != nil || failedFacts.Runners[len(failedFacts.Runners)-1].CoordinatorMode != "team" {
		t.Fatalf("coordinator facts did not fail closed: %v", err)
	}
	endpoint := model.Capability{Identity: model.CapabilityIdentity{SSH: &model.CapabilitySSH{Hostname: "coordinator.example"}}}
	matched, resolveErr := coordinatorNode(context.Background(), Deps{ResolveHost: func(context.Context, string) ([]string, error) { return []string{"127.0.0.1"}, nil }}, endpoint, "different-machine", map[string]bool{"127.0.0.1": true})
	if resolveErr != nil || !matched {
		t.Fatal("loopback endpoint escaped coordinator host")
	}
	if err := Register(Contributor{Name: "rogue", Field: "Pending", Fn: func(_ context.Context, w *sched.World) error { w.Counts.Global++; return nil }}); err != nil {
		t.Fatal(err)
	}
	partial, _, err := Build(context.Background(), d, p)
	if err == nil || !strings.Contains(err.Error(), "outside") || len(partial.Runners) != 0 {
		t.Fatalf("field ownership: %+v %v", partial, err)
	}
}
