package inventory

// Purpose: tests of Sync (P7-AUTO-04) against a real SQLite store and registry: candidates, label and
//          rank application that survives a store reopen, skip_repos, public_only, idempotence, the
//          trust fence, rejections that do not abort, dry-run and the TEAM refusal.
// Inputs:  the laptop capability fixture registered under several node ids.
// Outputs: pass or fail.
// Constraints: no network, no processes, no sleeps.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type fixture struct {
	path string
	st   *store.Store
	reg  *registry.Registry
}

func openFixture(t *testing.T, path string) *fixture {
	t.Helper()
	st, err := store.Open(path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return &fixture{path: path, st: st, reg: registry.New(st)}
}

// newFixture registers the given enrolled nodes, each authorized on /src/pub and /src/priv.
func newFixture(t *testing.T, nodes ...string) *fixture {
	t.Helper()
	f := openFixture(t, filepath.Join(t.TempDir(), "state.db"))
	b, err := os.ReadFile("../../model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range nodes {
		var c model.Capability
		if err = json.Unmarshal(b, &c); err != nil {
			t.Fatal(err)
		}
		c.Identity.ID, c.Identity.Name = id, id
		ctx := context.Background()
		if _, err = f.reg.Register(ctx, c, "operator"); err != nil {
			t.Fatal(err)
		}
		for _, p := range []string{"/src/pub", "/src/priv", "/src/old"} {
			if _, err = f.reg.Authorize(ctx, id, p, "operator"); err != nil {
				t.Fatal(err)
			}
		}
	}
	return f
}

func (f *fixture) env(vis Visibility) Env {
	return Env{Store: f.st, Registry: f.reg, Actor: "cli:test", Visibility: vis}
}

func (f *fixture) audit(t *testing.T) int {
	t.Helper()
	rows, err := f.st.AuditEntries(context.Background(), 0, 100000)
	if err != nil {
		t.Fatal(err)
	}
	return len(rows)
}

func rankOf(n int) *int { return &n }

func threeEntries() []Entry {
	return []Entry{
		{ID: "mini1", Rank: rankOf(1), Labels: []string{"xcode"}, Pools: []string{"mine"}, SkipRepos: []string{"/src/old"}},
		{ID: "vps1", SSH: "ci@vps1.example.test", OS: "linux", Arch: "amd64", Rank: rankOf(3), Pools: []string{"mine"}},
		{ID: "vps2", SSH: "vps2.example.test"},
	}
}

func TestSyncAppliesFixture(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, "mini1")
	before, _ := f.reg.Get(ctx, "mini1")
	sum, err := Sync(ctx, f.env(nil), threeEntries(), Options{})
	if err != nil || len(sum.Rejected) != 0 {
		t.Fatalf("sync: %+v %v", sum, err)
	}
	if !reflect.DeepEqual(sum.Accepted, []string{"mini1"}) || len(sum.Candidates) != 2 || !sum.Candidates[0].Created {
		t.Fatalf("summary: %+v", sum)
	}
	for _, id := range []string{"vps1", "vps2"} {
		n, err := f.reg.Get(ctx, id)
		if err != nil || !n.Capability.Lifecycle.Discovered || n.Capability.Lifecycle.Eligible || len(n.Capability.Lifecycle.AuthorizedProjects) != 0 ||
			len(n.Capability.Identity.HostKeyFingerprints) != 0 || n.Capability.SecretsEligible {
			t.Fatalf("candidate %s: %+v %v", id, n.Capability, err)
		}
		if ls, _ := f.st.NodeLabels(ctx, id); len(ls) != 0 {
			t.Fatalf("a candidate gets no labels yet: %v", ls)
		}
	}
	// Labels, rank and skip_repos reach the existing node; the rank survives a reopen of the store.
	st2 := openFixture(t, f.path)
	ls, _ := st2.st.NodeLabels(ctx, "mini1")
	var got []string
	for _, l := range ls {
		got = append(got, l.Key+"="+l.Value)
	}
	if !reflect.DeepEqual(got, []string{"pool.mine=", "rank=1", "xcode="}) {
		t.Fatalf("labels after reopen = %v", got)
	}
	after, _ := st2.reg.Get(ctx, "mini1")
	if !reflect.DeepEqual(after.Capability.Lifecycle.AuthorizedProjects, []string{"/src/pub", "/src/priv"}) {
		t.Fatalf("projects = %v", after.Capability.Lifecycle.AuthorizedProjects)
	}
	// Nothing else moved.
	after.Capability.Lifecycle.AuthorizedProjects = before.Capability.Lifecycle.AuthorizedProjects
	if !reflect.DeepEqual(before.Capability.Trust, after.Capability.Trust) || before.Capability.SecretsEligible != after.Capability.SecretsEligible {
		t.Fatal("trust or secrets eligibility changed")
	}
}

func TestSyncIdempotent(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, "mini1")
	if _, err := Sync(ctx, f.env(nil), threeEntries(), Options{}); err != nil {
		t.Fatal(err)
	}
	rows := f.audit(t)
	sum, err := Sync(ctx, f.env(nil), threeEntries(), Options{})
	if err != nil || len(sum.Changed) != 0 || len(sum.Rejected) != 0 {
		t.Fatalf("second run: %+v %v", sum, err)
	}
	if want := []Candidate{{ID: "vps1"}, {ID: "vps2"}}; !reflect.DeepEqual(sum.Candidates, want) {
		t.Fatalf("pending candidates = %+v", sum.Candidates)
	}
	if got := f.audit(t); got != rows {
		t.Fatalf("audit rows %d -> %d on an identical run", rows, got)
	}
}

func TestSyncNeverTouchesTrust(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, "mini1")
	before, _ := f.reg.Get(ctx, "mini1")
	e := Entry{ID: "mini1", Rank: rankOf(5), Labels: []string{"a=b"}, Pools: []string{"mine"}, PublicOnly: true, SkipRepos: []string{"/src/old"}}
	if _, err := Sync(ctx, f.env(nil), []Entry{e}, Options{}); err != nil {
		t.Fatal(err)
	}
	after, _ := f.reg.Get(ctx, "mini1")
	b, a := before.Capability, after.Capability
	if !reflect.DeepEqual(b.Trust, a.Trust) || !reflect.DeepEqual(b.Identity, a.Identity) || b.SecretsEligible != a.SecretsEligible ||
		!reflect.DeepEqual(b.Separation, a.Separation) || b.Lifecycle.Eligible != a.Lifecycle.Eligible || b.Lifecycle.Discovered != a.Lifecycle.Discovered {
		t.Fatalf("operator-owned fields only:\nbefore %+v\nafter  %+v", b, a)
	}
	// An existing node's ssh, os and arch in the inventory are ignored.
	e = Entry{ID: "mini1", SSH: "evil@elsewhere.example.test", OS: "windows", Arch: "arm64"}
	if sum, _ := Sync(ctx, f.env(nil), []Entry{e}, Options{}); len(sum.Changed) != 0 {
		t.Fatalf("identity facts changed: %+v", sum)
	}
	again, _ := f.reg.Get(ctx, "mini1")
	if !reflect.DeepEqual(again.Capability.Identity, a.Identity) || !reflect.DeepEqual(again.Capability.Platform, a.Platform) {
		t.Fatal("identity or platform moved")
	}
}

func TestPublicOnlyAuthorization(t *testing.T) {
	ctx := context.Background()
	e := Entry{ID: "mini1", PublicOnly: true}
	calls := map[string]int{}
	vis := func(_ context.Context, p string) (bool, bool) { calls[p]++; return p == "/src/pub", p != "/src/old" }
	f := newFixture(t, "mini1", "mini2")
	sum, err := Sync(ctx, f.env(vis), []Entry{e, {ID: "mini2", PublicOnly: true}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"mini1", "mini2"} {
		n, _ := f.reg.Get(ctx, id)
		if !reflect.DeepEqual(n.Capability.Lifecycle.AuthorizedProjects, []string{"/src/pub"}) {
			t.Fatalf("%s projects = %v", id, n.Capability.Lifecycle.AuthorizedProjects)
		}
	}
	if calls["/src/pub"] != 1 || calls["/src/priv"] != 1 || len(sum.Notes) != 1 {
		t.Fatalf("visibility read once per project: %v notes %v", calls, sum.Notes)
	}
	// The API is down: nothing is authorized and the summary says so.
	f = newFixture(t, "mini1")
	down := func(context.Context, string) (bool, bool) { return false, false }
	sum, _ = Sync(ctx, f.env(down), []Entry{e}, Options{})
	n, _ := f.reg.Get(ctx, "mini1")
	if len(n.Capability.Lifecycle.AuthorizedProjects) != 0 || len(sum.Notes) != 3 || len(sum.Changed) != 3 {
		t.Fatalf("unknown visibility: %+v projects %v", sum, n.Capability.Lifecycle.AuthorizedProjects)
	}
	// Without a reader the answer is unknown too; without public_only nothing is read.
	f = newFixture(t, "mini1")
	if sum, _ = Sync(ctx, f.env(nil), []Entry{{ID: "mini1", Rank: rankOf(1)}}, Options{}); len(sum.Notes) != 0 {
		t.Fatalf("notes without public_only: %v", sum.Notes)
	}
	if n, _ = f.reg.Get(ctx, "mini1"); len(n.Capability.Lifecycle.AuthorizedProjects) != 3 {
		t.Fatal("projects changed without public_only")
	}
}

func TestSyncRejectsWithoutAborting(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, "mini1")
	in := []Entry{{ID: "BAD"}, {ID: "newhost"}, {ID: "mini1", Labels: []string{"bad label"}}, {ID: "mini1", Rank: rankOf(1)}, {ID: "vps1", SSH: "vps1.example.test"}}
	sum, err := Sync(ctx, f.env(nil), in, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range sum.Rejected {
		ids = append(ids, r.ID)
	}
	if !slices.Equal(ids, []string{"BAD", "newhost", "mini1", "mini1"}) || len(sum.Candidates) != 1 || sum.Candidates[0].ID != "vps1" {
		t.Fatalf("summary: %+v", sum)
	}
	if ls, _ := f.st.NodeLabels(ctx, "mini1"); len(ls) != 0 {
		t.Fatalf("a rejected node got labels: %v", ls)
	}
}

func TestSyncDryRunWritesNothing(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, "mini1")
	rows := f.audit(t)
	sum, err := Sync(ctx, f.env(nil), threeEntries(), Options{DryRun: true})
	if err != nil || !sum.DryRun || len(sum.Changed) == 0 || len(sum.Candidates) != 2 || sum.Candidates[0].Created {
		t.Fatalf("dry run: %+v %v", sum, err)
	}
	if _, err = f.reg.Get(ctx, "vps1"); err == nil {
		t.Fatal("dry-run created a candidate")
	}
	if ls, _ := f.st.NodeLabels(ctx, "mini1"); len(ls) != 0 || f.audit(t) != rows {
		t.Fatalf("dry-run wrote: %v", ls)
	}
}

func TestSyncRefusedOnTeamCoordinator(t *testing.T) {
	f := newFixture(t, "mini1")
	env := f.env(nil)
	env.Team = true
	if _, err := Sync(context.Background(), env, threeEntries(), Options{}); ReasonOf(err) != ReasonTeam {
		t.Fatalf("team: %v", err)
	}
	if n, _ := f.reg.List(context.Background()); len(n) != 1 {
		t.Fatal("a refused sync registered nodes")
	}
}
