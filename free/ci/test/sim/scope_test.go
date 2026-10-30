package sim

// Purpose: A10 scoped placement and private project authorization fixtures.
// Inputs: personal and team pools, the real node project editor and dispatcher.
// Outputs: placement, delivery and authorization assertions without external services.
// Constraints: no sleeps, fixed ports or credentials.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/coordclient"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/labels"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/pools"
	"github.com/nself-org/plugins/free/ci/internal/pools/record"
	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/where"
)

func TestScopePersonalVsTeamPool(t *testing.T) {
	runners := []pools.Runner{
		{ID: "vps", Source: pools.SourceRegistry, Assigned: []string{"pool.mine"}, Projects: []string{"private"}},
		{ID: "team-node", Source: pools.SourceCoordinator, Assigned: []string{"pool.team"}, Projects: []string{"private"}},
	}
	rec := pools.List{Set: true, IDs: []string{"team"}}
	excluded := record.Excluded(rec, runners)
	if excluded["vps"][0].Detail != record.Reason || len(excluded["team-node"]) != 0 {
		t.Fatalf("commit CI pool rule: %v", excluded)
	}
	for _, tc := range []struct {
		dev  []string
		want bool
	}{{[]string{"team"}, false}, {[]string{"mine", "team"}, true}} {
		in := pools.Input{Runners: runners, Project: "private", Dev: true, DevPools: pools.List{Set: true, IDs: tc.dev}}
		got := pools.Resolve(in)
		if slices.Contains(got.Eligible, "vps") != tc.want {
			t.Fatalf("dev pools %v: %+v", tc.dev, got)
		}
		if !tc.want && got.Excluded["vps"][0].Detail != pools.DetailDevNotAllowed {
			t.Fatal(got.Excluded)
		}
	}
}

func scopeStore(t *testing.T, f *fixture) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(f.home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func scopePolicy(t *testing.T, st *store.Store, key, raw string) {
	t.Helper()
	err := st.ApplyWrite(context.Background(), store.PolicyWrite{Op: store.PolicySet, Scope: "user", Subject: "local", Key: key, Value: json.RawMessage(raw), Actor: "test"}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func scopeRun(t *testing.T, f *fixture, rec bool) run.Result {
	t.Helper()
	pl, err := plan.Build(plan.Input{Config: model.PipelineConfig{Version: 1, Jobs: map[string]model.Job{"unit": remoteJob(0)}},
		World: plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, CPU: 8, MemMB: 16384}})
	if err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	c := &run.Coordinator{Opts: run.Options{Root: f.repo, Home: f.home, NoStatus: true, Record: rec}, Plan: pl, ConfigDigest: "cfg", Out: out, Capture: f.git,
		Capacity: func() ciexec.Capacity {
			return ciexec.Capacity{CPUs: 8, MemoryMB: 16384, FreeMemoryMB: 12000, BatteryPercent: -1, PluggedIn: true, Interactive: true}
		}}
	res, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v %s", err, out)
	}
	return res
}

func TestRecordPoolTransportExclusion(t *testing.T) {
	r := passRunner("scope")
	f := newFixture(t, map[string]func(context.Context, *node, session.JobRun) (session.JobResult, error){"n1": r, "n2": r})
	st := scopeStore(t, f)
	for id, pool := range map[string]string{"n1": "mine", "n2": "team"} {
		if err := st.NodeLabelsReplace(context.Background(), id, []store.NodeLabel{{Key: "pool." + pool}}, "test"); err != nil {
			t.Fatal(err)
		}
	}
	scopePolicy(t, st, record.Key, `["team"]`)
	res := scopeRun(t, f, true)
	if res.Exit != 0 {
		t.Fatalf("record run failed: %+v", res.Summary)
	}
	if len(f.nodes[0].ran()) != 0 || f.nodes[0].sessions.Load() != 0 || len(f.nodes[1].ran()) != 1 {
		t.Fatal("a non-record runner received a checkout or lease")
	}
	attempts, err := st.PipelineAttempts(context.Background(), res.RunID)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range attempts {
		facts, ok, err := (store.AttestLedger{S: st}).AttemptFacts(context.Background(), a.ID)
		if err != nil || !ok || facts.RunnerID != "n2" {
			t.Fatalf("lease holder %q %v %v", facts.RunnerID, ok, err)
		}
	}
	scopePolicy(t, st, pools.KeyDev, `["mine"]`)
	if res := scopeRun(t, f, false); res.Exit != 0 || len(f.nodes[0].ran()) != 1 {
		t.Fatalf("dev run could not use the allowed personal VPS: %+v", res.Summary)
	}
	scopePolicy(t, st, pools.KeyDev, `["team"]`)
	if res := scopeRun(t, f, false); res.Exit != 0 || len(f.nodes[0].ran()) != 1 || len(f.nodes[1].ran()) != 2 {
		t.Fatalf("dev run ignored pools.dev: %+v", res.Summary)
	}
}

func TestPersonalNodePrivateRepoOptIn(t *testing.T) {
	r := passRunner("private")
	f := newFixture(t, map[string]func(context.Context, *node, session.JobRun) (session.JobResult, error){"n1": r, "n2": r})
	st := scopeStore(t, f)
	reg := registry.New(st)
	if _, err := reg.Deauthorize(context.Background(), "n1", f.repo, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Deauthorize(context.Background(), "n2", f.repo, "test"); err != nil {
		t.Fatal(err)
	}
	res := scopeRun(t, f, false)
	if res.Exit == 0 || len(f.nodes[0].ran()) != 0 || f.nodes[0].sessions.Load() != 0 {
		t.Fatal("unauthorized personal node received private source")
	}
	// This is the same editor `nodes projects add` calls, with a directory resolver.
	editor := labels.Env{Store: st, Registry: reg, Actor: "test", ResolveDir: func(context.Context, string) (string, bool) { return f.repo, true }}
	if _, err := editor.AddProjects(context.Background(), "n1", []string{f.repo}, false); err != nil {
		t.Fatal(err)
	}
	res = scopeRun(t, f, false)
	if res.Exit != 0 || len(f.nodes[0].ran()) != 1 {
		t.Fatalf("authorized personal node ineligible: %+v", res.Summary)
	}
}

func TestTeamPoolViaCoordinator(t *testing.T) {
	r := &coordclient.Resolver{Inputs: func() coordclient.Inputs {
		return coordclient.Inputs{Flag: "https://team.example.invalid", Home: t.TempDir(), Getenv: func(string) string { return "" }}
	}, Warn: io.Discard}
	target, err := r.Target()
	if err != nil || target.Kind != coordclient.Remote || !r.Active() {
		t.Fatalf("TEAM target was not remote: %v %v", target, err)
	}
	targetPool, err := where.Values("team", where.Known{Pools: []string{"mine", "team"}, Effective: []string{"team"}}, where.Opts{})
	if err != nil || targetPool.Kind != where.KindPool || string(targetPool.Values[pools.KeyAllow]) != `["team"]` {
		t.Fatalf("team pool was not selected: %+v %v", targetPool, err)
	}
	// An unreachable/unconfigured coordinator must remain authoritative, never fall back locally.
	_, _, active, err := r.Client()
	if err == nil || !active {
		t.Fatal("remote target fell back to the personal engine")
	}
}
