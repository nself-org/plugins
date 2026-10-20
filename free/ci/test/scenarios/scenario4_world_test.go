//go:build scenarios

package scenarios

// Purpose: Epic acceptance 2 through the REAL world builder. Three nodes (linux/amd64, linux/arm64,
// darwin/arm64) are registered in the store's node registry; world.Build assembles the snapshot the
// dispatcher plans from; sched.Place puts each matrix entry on the node of its own platform.
// Inputs:  a real store, world.Build with a policy hook, sched.Place.
// Outputs: assertions only.
// Constraints: no probe, no network; the node documents are built with planview.LocalCapability, so
//          they pass the capability schema exactly as a registered node's document does.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

// assigned is a fact the coordinator decided, as the registry stores it.
func assigned[T any](v T) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: "assigned", ObservedAt: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC), Confidence: "known"}
}

// putNode registers an online LAN node of the given platform.
func putNode(t *testing.T, st *store.Store, id, plat string) {
	t.Helper()
	c, err := planview.LocalCapability(planview.LocalFacts{Project: "project", Platform: plat, CPUs: 8, MemMB: 8192, Now: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	c.Identity.ID, c.Identity.Name, c.Identity.Transport = id, id, "ssh"
	c.Location.Kind.Value = ptr("lan")
	c.Lifecycle.Eligible = true
	c.Lifecycle.AuthorizedProjects = []string{"project"}
	// what the coordinator assigns a trusted operator node (the trust declaration the world maps into admission)
	c.Trust.Accepts = assigned([]model.TrustClass{"owner", "internal", "collaborator", "untrusted"})
	c.Trust.Isolation = assigned(model.Isolation("vm"))
	c.Trust.Network = assigned([]model.NetworkScope{"privileged"})
	c.Trust.PrivacyZone = assigned(model.PrivacyZone("local-only"))
	c.Identity.Ownership = assigned("operator")
	c.Separation.UIDSeparation = assigned(true)
	doc, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := store.NewAdminAuthority("scenarios-admin")
	if err != nil {
		t.Fatal(err)
	}
	// NodeAdd is the one write that accepts trust and lifecycle: what `nodes add` does for an admin
	if err = st.NodeAdd(context.Background(), store.NodeRecord{ID: id, Name: id, State: "online", Capability: doc}, admin); err != nil {
		t.Fatal(err)
	}
}

func TestScenario4RealWorldPlacesEachEntryOnItsPlatform(t *testing.T) {
	st := openStore(t)
	nodes := map[string]string{"node-amd": "linux/amd64", "node-arm": "linux/arm64", "node-mac": "darwin/arm64"}
	for id, plat := range nodes {
		putNode(t, st, id, plat)
	}
	pol := func(context.Context, string) (sched.Policy, error) {
		return sched.Policy{Preset: "balanced", Trust: trust.Effective{}}, nil
	}
	w, digest, err := world.Build(context.Background(), world.Deps{Store: st, Policy: pol, Clock: newClock().Now}, sched.Pipeline{ID: "pipe", Support: matrixPlatforms})
	if err != nil || digest == "" {
		t.Fatalf("world.Build: %v (digest %q)", err, digest)
	}
	if len(w.Runners) != len(nodes) {
		t.Fatalf("the world holds %d runners, want %d", len(w.Runners), len(nodes))
	}
	jobs := make([]sched.Job, 0, len(matrixPlatforms))
	for _, p := range matrixPlatforms {
		j := job("build-"+strings.ReplaceAll(p, "/", "-"), p)
		j.Pipeline.Support = matrixPlatforms
		jobs = append(jobs, j)
	}
	res := sched.Place(w, jobs)
	if res.ErrorCode != "" || len(res.Placements) != len(nodes) {
		t.Fatalf("placement: %+v", res)
	}
	for _, p := range res.Placements {
		want := ""
		for id, plat := range nodes {
			if "build-"+strings.ReplaceAll(plat, "/", "-") == p.JobKey {
				want = id
			}
		}
		if p.RunnerID != want {
			t.Fatalf("%s landed on %s, want %s", p.JobKey, p.RunnerID, want)
		}
	}
	// removing one platform's node turns that entry into a wait/refusal, never a misplacement
	st2 := openStore(t)
	putNode(t, st2, "node-amd", "linux/amd64")
	putNode(t, st2, "node-arm", "linux/arm64")
	w2, _, err := world.Build(context.Background(), world.Deps{Store: st2, Policy: pol, Clock: newClock().Now}, sched.Pipeline{ID: "pipe", Support: matrixPlatforms})
	if err != nil {
		t.Fatal(err)
	}
	res2 := sched.Place(w2, jobs)
	if len(res2.Placements) != 2 {
		t.Fatalf("with darwin/arm64 missing want 2 placements, got %+v", res2.Placements)
	}
}

// TestScenario4RealWorldDispatch drives the three entries through the dispatcher with world.Build as
// its world source: the node rows in the registry are the only runners, so a placement on the wrong
// platform would show in the store's admission record.
func TestScenario4RealWorldDispatch(t *testing.T) {
	st := openStore(t)
	nodes := map[string]string{"node-amd": "linux/amd64", "node-arm": "linux/arm64", "node-mac": "darwin/arm64"}
	for id, plat := range nodes {
		putNode(t, st, id, plat)
	}
	clk := newClock()
	pol := func(context.Context, string) (sched.Policy, error) { return sched.Policy{Preset: "balanced"}, nil }
	build := func(ctx context.Context, p sched.Pipeline) (sched.World, string, error) {
		return world.Build(ctx, world.Deps{Store: st, Policy: pol, Clock: clk.Now}, p)
	}
	tr := &transport{}
	d := newDispatcher(t, dispatch.Config{Store: st, World: build, Poll: time.Millisecond, Transport: tr, Clock: clk.Now})
	for id, plat := range nodes {
		attempt := queueAttempt(t, st, "pipe", "project", "build-"+strings.ReplaceAll(plat, "/", "-"))
		j := dispatch.Job{AttemptID: attempt, Sched: job(attempt, plat)}
		j.Sched.Pipeline.Support = matrixPlatforms
		a, err := d.Submit(context.Background(), j)
		if err != nil {
			t.Fatalf("%s: %v", plat, err)
		}
		if a.RunnerID != id {
			t.Fatalf("%s landed on %s, want %s", plat, a.RunnerID, id)
		}
	}
	d.Close()
	if n := len(tr.deliveries()); n != len(nodes) {
		t.Fatalf("%d deliveries, want %d", n, len(nodes))
	}
}
