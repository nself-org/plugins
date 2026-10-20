package mode

// Purpose: TestCoordinatorHostFloor (P7-NODE-20, review F4): serve -> coordinator facts -> world ->
// Declaration -> boundary.coordinator_host, and the refusal to start without the facts provider.
// Inputs:  a temporary store, the capability fixture of model/testdata, the real world.Build and
//          trust.Admissible.
// Outputs: pass or fail.
// Constraints: no process is spawned and no port is bound; the serve is the mode.Start check plus Begin,
//          which is exactly what cmd/serve_mode.go does around the V15 serve.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

// localRunner builds the world and returns the injected local runner.
func localRunner(t *testing.T, st *store.Store) sched.Runner {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "model", "testdata", "capability", "valid", "laptop.valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	process := model.Isolation("process")
	c.Trust.Isolation.Value = &process
	d := world.Deps{Store: st, Local: &c,
		Policy:         func(context.Context, string) (sched.Policy, error) { return sched.Policy{}, nil },
		Clock:          func() time.Time { return time.Unix(100, 0) },
		LocalAddresses: func(context.Context) ([]string, error) { return []string{"127.0.0.1"}, nil },
		ResolveHost:    func(context.Context, string) ([]string, error) { return nil, nil },
	}
	w, _, err := world.Build(context.Background(), d, sched.Pipeline{ID: "pipe"})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range w.Runners {
		if r.ID == "local" {
			return r
		}
	}
	t.Fatal("the world has no local runner")
	return sched.Runner{}
}

// floorRefused reports whether the runner's Declaration refuses a job of the trust class with
// boundary.coordinator_host.
func floorRefused(r sched.Runner, class model.TrustClass) bool {
	job := trust.JobFacts{Trust: class, Network: "restricted", PersonalOwnRun: class == "owner"}
	for _, reason := range trust.Admissible(job, r.Decl, trust.Effective{}) {
		if reason == trust.BoundaryCoordinatorHost {
			return true
		}
	}
	return false
}

func TestCoordinatorHostFloor(t *testing.T) {
	ctx := context.Background()
	if err := RegisterProvider(); err != nil {
		t.Fatal(err)
	}
	if err := RegisterProvider(); err != nil {
		t.Fatalf("a second call must be a no-op: %v", err)
	}
	if !world.CoordinatorFactsRegistered() {
		t.Fatal("the provider is not registered with the world")
	}
	open := func(t *testing.T) *store.Store {
		t.Helper()
		st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = st.Close() })
		return st
	}
	t.Run("no serve: the local runner is not a coordinator host", func(t *testing.T) {
		r := localRunner(t, open(t))
		if r.CoordinatorHost || floorRefused(r, "collaborator") || floorRefused(r, "owner") {
			t.Fatalf("a plain run must not meet the floor: %+v", r.Decl)
		}
	})
	t.Run("personal serve: owner process job runs, collaborator job is refused", func(t *testing.T) {
		st := open(t)
		d, err := Start(ctx, st, false, "operator:serve")
		if err != nil || d.Mode != Personal {
			t.Fatalf("%+v %v", d, err)
		}
		s := Begin(Options{Store: st})
		r := localRunner(t, st)
		s.End()
		if !r.CoordinatorHost || r.CoordinatorMode != trust.Personal {
			t.Fatalf("serve facts did not reach the world: host=%v mode=%q", r.CoordinatorHost, r.CoordinatorMode)
		}
		if floorRefused(r, "owner") {
			t.Fatal("the owner's own process-level job must run on a personal coordinator host")
		}
		for _, class := range []model.TrustClass{"collaborator", "untrusted"} {
			if !floorRefused(r, class) {
				t.Fatalf("%s job on a personal coordinator host must meet boundary.coordinator_host", class)
			}
		}
		if after := localRunner(t, st); after.CoordinatorHost {
			t.Fatal("the floor outlived the serve")
		}
	})
	t.Run("team serve: the owner job is refused too", func(t *testing.T) {
		st := open(t)
		d, err := Start(ctx, st, true, "operator:serve")
		if err != nil || d.Mode != Team {
			t.Fatalf("%+v %v", d, err)
		}
		s := Begin(Options{Store: st})
		defer s.End()
		r := localRunner(t, st)
		if !r.CoordinatorHost || r.CoordinatorMode != trust.Team {
			t.Fatalf("host=%v mode=%q", r.CoordinatorHost, r.CoordinatorMode)
		}
		for _, class := range []model.TrustClass{"owner", "collaborator"} {
			if !floorRefused(r, class) {
				t.Fatalf("%s job on a team coordinator host must meet boundary.coordinator_host", class)
			}
		}
	})
	t.Run("ci serve refuses to start without the provider", func(t *testing.T) {
		for name, registered := range map[string]func() bool{"nil": nil, "false": func() bool { return false }} {
			err := RequireProvider(registered)
			if err == nil || ReasonOf(err) != ReasonFactsProviderMissing {
				t.Fatalf("%s: want facts_provider_missing, got %v", name, err)
			}
		}
		if err := RequireProvider(world.CoordinatorFactsRegistered); err != nil {
			t.Fatalf("a registered provider must pass: %v", err)
		}
	})
}
