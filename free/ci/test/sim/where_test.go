package sim

// Purpose: P7-AUTO-06 proof: `--where <node>` places on that node, `--where
// local` never on a registered node, `--where hosted` with no provider fails
// fast with its reasons. The world is the one `ci plan` builds from a store
// of registered nodes; the run-scope values come from internal/where.
// Inputs:  an in-memory-sized state.db in t.TempDir(). Outputs: pass or fail.
// Constraints: pure Go, no processes, no network, no clock reads in decisions.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
	"github.com/nself-org/plugins/free/ci/internal/where"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

func whereFact[T any](v T) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: "assigned", ObservedAt: time.Now(), Confidence: "known"}
}

// whereNode registers one online linux LAN node the owner's jobs may use.
func whereNode(t *testing.T, st *store.Store, id string) {
	t.Helper()
	ctx := context.Background()
	raw, err := os.ReadFile("../../internal/model/testdata/capability/valid/vps.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	lan, owned, uid, slots := "lan", "owned", true, 4
	c.Identity.ID, c.Identity.Name, c.Identity.Labels = id, id, nil
	c.Location.Kind.Value, c.Economics.Class.Value = &lan, &owned
	c.Separation.UIDSeparation = model.Fact[bool]{Value: &uid, Source: "self", ObservedAt: time.Now(), Confidence: "known"}
	c.Availability.Slots.Value = &slots
	c.Lifecycle = model.CapabilityLifecycle{AuthorizedProjects: []string{}}
	doc, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.NodePut(ctx, store.NodeRecord{ID: id, Name: id, State: "offline", Lifecycle: []byte("{}"), Trust: []byte("{}"), DeployHost: []byte("{}"), Capability: doc, ObservedAt: time.Now().UnixNano()}, "test"); err != nil {
		t.Fatal(err)
	}
	admin, err := store.NewAdminAuthority("test")
	if err != nil {
		t.Fatal(err)
	}
	reg := registry.New(st)
	tr := model.CapabilityTrust{
		Accepts:       whereFact([]model.TrustClass{"owner", "internal"}),
		Isolation:     whereFact(model.Isolation("container")),
		Network:       whereFact([]model.NetworkScope{"none", "internet", "restricted", "lan", "privileged"}),
		SecretClasses: whereFact([]model.SecretClass{"none"}),
		PrivacyZone:   whereFact(model.PrivacyZone("private-infrastructure")),
	}
	if _, err = reg.SetTrust(ctx, id, tr, admin); err != nil {
		t.Fatal(err)
	}
	if _, err = reg.SetState(ctx, id, "online", "", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err = reg.Authorize(ctx, id, "p", "test"); err != nil {
		t.Fatal(err)
	}
}

// wherePlace places one job under the run-scope values of a --where target.
func wherePlace(t *testing.T, st *store.Store, vals map[string]json.RawMessage) sched.Result {
	t.Helper()
	reg, err := trust.NewRegistry(policy.Default.Keys()...)
	if err != nil {
		t.Fatal(err)
	}
	eff, _, err := reg.Merge("", trust.Scope{Kind: trust.SourceRun, Values: vals})
	if err != nil {
		t.Fatal(err)
	}
	sp, err := schedpolicy.Placement(&eff)
	if err != nil {
		t.Fatal(err)
	}
	job := sched.Job{Key: "j", ID: "j", Project: "p", Pipeline: sched.Pipeline{ID: "pipe", Support: []string{"linux/arm64"}},
		Requirements: model.PlacementRequirements{Platform: "linux/arm64", CPU: 1, MemMB: 128},
		Trust:        "owner", Network: "lan", TimeoutMs: 60000, PersonalOwnRun: true}
	deps := world.Deps{Store: st, Policy: func(context.Context, string) (sched.Policy, error) { return sp, nil }, Clock: time.Now}
	wd, _, err := world.Build(context.Background(), deps, job.Pipeline)
	if err != nil {
		t.Fatal(err)
	}
	return sched.Place(wd, []sched.Job{job})
}

func whereStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	whereNode(t, st, "lan1")
	whereNode(t, st, "lan2")
	return st
}

func TestWhereNodeSim(t *testing.T) {
	st := whereStore(t)
	k := where.Known{Nodes: []string{"lan1", "lan2"}}
	tgt, err := where.Values("lan2", k, where.Opts{})
	if err != nil {
		t.Fatal(err)
	}
	res := wherePlace(t, st, tgt.Values)
	if len(res.Placements) != 1 || res.Placements[0].RunnerID != "lan2" {
		t.Fatalf("--where lan2 placed %+v", res)
	}
}

func TestWhereLocalNeverRemote(t *testing.T) {
	st := whereStore(t)
	tgt, err := where.Values("local", where.Known{Nodes: []string{"lan1", "lan2"}}, where.Opts{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range wherePlace(t, st, tgt.Values).Placements {
		if p.RunnerID == "lan1" || p.RunnerID == "lan2" {
			t.Fatalf("--where local placed on registered node %s", p.RunnerID)
		}
	}
}

func TestWhereHostedNoProviderFailsFast(t *testing.T) {
	_, err := where.Values("hosted", where.Known{Nodes: []string{"lan1"}}, where.Opts{})
	we, ok := err.(*where.Error)
	if !ok || len(we.Reasons) == 0 {
		t.Fatalf("want an E797 with a reason list, got %v", err)
	}
}
