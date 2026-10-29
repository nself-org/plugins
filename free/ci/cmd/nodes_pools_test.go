package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/autoworld"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pools"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// poolsFixture is a personal VPS in pool mine, a team coordinator node in pool team, this machine and a provider.
func poolsFixture(t *testing.T, allow string) poolsDeps {
	t.Helper()
	reg, err := trust.NewRegistry(pools.Keys()...)
	if err != nil {
		t.Fatal(err)
	}
	vals := map[string]json.RawMessage{}
	if allow != "" {
		vals[pools.KeyAllow] = json.RawMessage(allow)
	}
	eff, _, err := reg.Merge("", trust.Scope{Kind: trust.SourceProtectedBranch, Values: vals})
	if err != nil {
		t.Fatal(err)
	}
	w := poolsWorld{Project: "/src/app", Policy: eff, Runners: []pools.Runner{
		{ID: "vps", Source: pools.SourceRegistry, Location: "remote", Assigned: []string{"pool.mine"}, Projects: []string{"/src/app"}},
		{ID: "t1", Source: pools.SourceCoordinator, Location: "lan", Assigned: []string{"pool.team"}, Projects: []string{"/src/app"}},
		{ID: "t2", Source: pools.SourceCoordinator, Location: "lan", Assigned: []string{"pool.team"}},
		{ID: "local", Source: pools.SourceLocal, Location: "local", Projects: []string{"/src/app"}},
		{ID: "gh", Source: pools.SourceProvider, Location: "hosted", Projects: []string{"/src/app"}}}}
	return poolsDeps{Load: func(context.Context, string) (poolsWorld, error) { return w, nil }}
}

func runPoolsJSON(t *testing.T, sub string, d poolsDeps, args ...string) map[string]json.RawMessage {
	t.Helper()
	var out, errOut bytes.Buffer
	if code := runNodesPools(sub, append(args, "--json"), d, &out, &errOut); code != 0 {
		t.Fatalf("%s exit %d: %s %s", sub, code, out.String(), errOut.String())
	}
	var env struct {
		Command string                     `json:"command"`
		Data    map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil || env.Command != "ci nodes pools "+sub {
		t.Fatalf("envelope %q: %v", out.String(), err)
	}
	return env.Data
}

func TestNodesPoolsListJSON(t *testing.T) {
	var ps []pools.Pool
	if err := json.Unmarshal(runPoolsJSON(t, "list", poolsFixture(t, ""))["pools"], &ps); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, p := range ps {
		ids = append(ids, p.ID)
		if p.Schema != pools.SchemaID || !p.Effective.Allowed {
			t.Fatalf("solo default: pool %+v", p)
		}
	}
	if !slices.Equal(ids, []string{"hosted", "local", "mine", "team"}) || ps[3].Scope != pools.ScopeProject || ps[2].Scope != pools.ScopeOperator {
		t.Fatalf("pools = %+v", ps)
	}
	if err := json.Unmarshal(runPoolsJSON(t, "show", poolsFixture(t, `["team"]`), "mine")["pools"], &ps); err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].Effective.Allowed || ps[0].Effective.Reasons[0].Detail != pools.DetailNotInProjectList {
		t.Fatalf("show mine = %+v", ps)
	}
	var out, errOut bytes.Buffer
	if code := runNodesPools("show", []string{"Bad_ID"}, poolsFixture(t, ""), &out, &errOut); code == 0 {
		t.Fatal("a malformed pool id was accepted")
	}
	if code := runNodesPools("list", []string{"extra"}, poolsFixture(t, ""), &out, &errOut); code == 0 {
		t.Fatal("list took a positional argument")
	}
}

func TestNodesPoolsExplain(t *testing.T) {
	d := poolsFixture(t, `["team"]`)
	data := runPoolsJSON(t, "explain", d, "--project", ".")
	var ex pools.Explanation
	raw, _ := json.Marshal(data)
	if err := json.Unmarshal(raw, &ex); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range ex.Excluded {
		got[r.RunnerID] = r.Reasons[0].Code + " " + r.Reasons[0].Detail
	}
	want := map[string]string{"vps": "pool.excluded not_in_project_list", "local": "pool.excluded not_in_project_list", "gh": "pool.excluded not_in_project_list"}
	if len(got) != len(want) {
		t.Fatalf("excluded = %v", got)
	}
	for id, w := range want {
		if got[id] != w {
			t.Fatalf("runner %s: %q, want %q", id, got[id], w)
		}
	}
	if !slices.Equal(ex.Eligible, []string{"t1"}) || !slices.Equal(ex.Unauthorized, []string{"t2"}) {
		t.Fatalf("eligible %v unauthorized %v", ex.Eligible, ex.Unauthorized)
	}
	// the plan path carries the same reasons: the registered provider over the same runners.
	if !slices.Contains(autoworld.Providers(), pools.ProviderName) {
		t.Fatalf("pools exclusion provider is not registered: %v", autoworld.Providers())
	}
	w, _ := d.Load(context.Background(), "")
	world := sched.World{Policy: sched.Policy{Trust: w.Policy}, Runners: []sched.Runner{{ID: "vps"}, {ID: "t1"}, {ID: "local"}}, Hosted: []sched.Runner{{ID: "gh"}}}
	lbl := poolsLabels{"vps": {{Key: "pool.mine"}}, "t1": {{Key: "pool.team"}}}
	plan, err := pools.ExclusionProvider(pools.Config{Labels: lbl})(context.Background(), world)
	if err != nil {
		t.Fatal(err)
	}
	for id, w := range want {
		if r := plan[id]; len(r) != 1 || r[0].Code+" "+r[0].Detail != w || r[0].Code != string(model.PoolExcluded) {
			t.Fatalf("plan reasons for %s = %v, want %q", id, r, w)
		}
	}
	var out, errOut bytes.Buffer
	broken := poolsDeps{Load: func(context.Context, string) (poolsWorld, error) {
		return poolsWorld{PolicyErr: errors.New("no repository")}, nil
	}}
	if code := runNodesPools("explain", nil, broken, &out, &errOut); code == 0 {
		t.Fatal("explain without a project policy succeeded")
	}
	out.Reset()
	if code := runNodesPools("list", nil, broken, &out, &errOut); code != 0 || !strings.Contains(out.String(), "every pool term is unset") {
		t.Fatalf("list without a project policy: %d %q", code, out.String())
	}
}

type poolsLabels map[string][]store.NodeLabel

func (f poolsLabels) NodeLabels(_ context.Context, id string) ([]store.NodeLabel, error) {
	return f[id], nil
}
