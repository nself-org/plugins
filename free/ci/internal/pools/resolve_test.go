package pools

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"slices"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

var propIDs = []string{"mine", "team", "ci", "gpu", Local, Hosted}

func randList(rng *rand.Rand) List {
	if rng.Intn(3) == 0 {
		return List{}
	}
	l := List{Set: true, IDs: []string{}}
	for _, id := range propIDs {
		if rng.Intn(2) == 0 {
			l.IDs = append(l.IDs, id)
		}
	}
	return l
}

func randInput(rng *rand.Rand) Input {
	in := Input{Allow: randList(rng), Enrolled: randList(rng), DevPools: randList(rng), Dev: rng.Intn(2) == 0}
	if rng.Intn(2) == 0 {
		in.Project = "p1"
	}
	sources := []string{SourceLocal, SourceRegistry, SourceCoordinator, SourceProvider}
	for i := range rng.Intn(6) {
		r := Runner{ID: fmt.Sprintf("r%d", i), Source: sources[rng.Intn(len(sources))], Location: "lan"}
		for _, id := range propIDs {
			if rng.Intn(3) == 0 {
				r.Assigned = append(r.Assigned, LabelPrefix+id)
			}
		}
		if rng.Intn(4) == 0 {
			r.Assigned = append(r.Assigned, "pool.Bad_ID", "region=eu")
		}
		if rng.Intn(2) == 0 {
			r.Projects = []string{"p1"}
		}
		in.Runners = append(in.Runners, r)
	}
	return in
}

// holds checks one term for one runner's remaining pools, the way the brief states the term.
func holds(ids []string, l List) bool {
	if !l.Set {
		return true
	}
	return slices.ContainsFunc(ids, l.Has)
}

func TestResolveIntersectionProperty(t *testing.T) {
	const seed = int64(20261027)
	rng := rand.New(rand.NewSource(seed))
	for n := range 10000 {
		in := randInput(rng)
		got := Resolve(in)
		for _, r := range in.Runners {
			ids := Memberships(r)
			// every term holds on the same pool: the effective set is the intersection of the set terms.
			var both []string
			for _, id := range ids {
				if holds([]string{id}, in.Allow) && holds([]string{id}, in.Enrolled) && (!in.Dev || holds([]string{id}, in.DevPools)) {
					both = append(both, id)
				}
			}
			anySet := in.Allow.Set || in.Enrolled.Set || in.Dev && in.DevPools.Set
			want := (!anySet || len(both) > 0) && (in.Project == "" || slices.Contains(r.Projects, in.Project))
			if slices.Contains(got.Eligible, r.ID) != want {
				t.Fatalf("seed %d case %d: runner %s eligible=%v, want %v (input %+v)", seed, n, r.ID, !want, want, in)
			}
			if in.Allow.Set && slices.Contains(got.Eligible, r.ID) && !holds(ids, in.Allow) {
				t.Fatalf("seed %d case %d: runner %s outside pools.allow is eligible", seed, n, r.ID)
			}
		}
		// removing any term never shrinks the set.
		for _, drop := range []func(*Input){func(i *Input) { i.Allow = List{} }, func(i *Input) { i.Enrolled = List{} }, func(i *Input) { i.DevPools = List{} }, func(i *Input) { i.Dev = false }, func(i *Input) { i.Project = "" }} {
			wider := in
			drop(&wider)
			more := Resolve(wider).Eligible
			for _, id := range got.Eligible {
				if !slices.Contains(more, id) {
					t.Fatalf("seed %d case %d: removing a term dropped runner %s", seed, n, id)
				}
			}
		}
	}
}

func TestSelfDeclaredLabelIgnored(t *testing.T) {
	if got := FromAssigned([]string{"pool.team", "pool.local", "pool.hosted", "pool.Bad", "pools.x", "team", "pool.ci=yes", "pool.team"}); !slices.Equal(got, []string{"ci", "team"}) {
		t.Fatalf("FromAssigned = %v", got)
	}
	in := Input{Allow: List{Set: true, IDs: []string{"team"}}, Runners: []Runner{{ID: "vps", Source: SourceRegistry}}}
	got := Resolve(in)
	if len(got.Eligible) != 0 || got.Excluded["vps"][0].Detail != DetailNotInProjectList {
		t.Fatalf("an unlabelled node joined pool team: %+v", got)
	}
}

func TestImplicitPools(t *testing.T) {
	in := Input{Runners: []Runner{{ID: "local", Source: SourceLocal, Location: "local"}, {ID: "gh-ubuntu", Source: SourceProvider, Location: "hosted"}}}
	got := Resolve(in)
	if len(got.Pools) != 2 || got.Pools[0].ID != Hosted || got.Pools[0].Scope != ScopeHosted || got.Pools[1].ID != Local || got.Pools[1].Scope != ScopeOperator {
		t.Fatalf("implicit pools = %+v", got.Pools)
	}
	in.Allow = List{Set: true, IDs: []string{Local}}
	got = Resolve(in)
	if !slices.Equal(got.Eligible, []string{"local"}) || got.Excluded["gh-ubuntu"][0].Code != string(model.PoolExcluded) {
		t.Fatalf("allow [local]: %+v", got)
	}
	in.Dev, in.DevPools = true, List{Set: true, IDs: []string{Hosted}}
	if got = Resolve(in); got.Excluded["local"][0].Detail != DetailDevNotAllowed {
		t.Fatalf("dev run on hosted only: %+v", got.Excluded)
	}
}

// TestVPSAndTeamFixture is the brief's fixture: a personal VPS in pool mine and a team coordinator pool team.
func TestVPSAndTeamFixture(t *testing.T) {
	runners := []Runner{{ID: "vps", Source: SourceRegistry, Location: "remote", Assigned: []string{"pool.mine"}},
		{ID: "t1", Source: SourceCoordinator, Location: "lan", Assigned: []string{"pool.team"}}}
	got := Resolve(Input{Runners: runners, Allow: List{Set: true, IDs: []string{"team"}}})
	if !slices.Equal(got.Eligible, []string{"t1"}) || got.Excluded["vps"][0].Detail != DetailNotInProjectList {
		t.Fatalf("project listing team: %+v", got)
	}
	if got = Resolve(Input{Runners: runners}); !slices.Equal(got.Eligible, []string{"t1", "vps"}) {
		t.Fatalf("solo default: %+v", got)
	}
	if got.Pools[0].ID != "mine" || got.Pools[0].Scope != ScopeOperator || got.Pools[1].Scope != ScopeProject {
		t.Fatalf("derived scopes: %+v", got.Pools)
	}
}

func TestRevisionOnlyTightens(t *testing.T) {
	reg, err := trust.NewRegistry(Keys()...)
	if err != nil {
		t.Fatal(err)
	}
	branch := trust.Scope{Kind: trust.SourceProtectedBranch, Values: map[string]json.RawMessage{KeyAllow: json.RawMessage(`["team","mine"]`)}}
	merge := func(rev string) (List, []string) {
		t.Helper()
		eff, _, err := reg.Merge("", branch, trust.Scope{Kind: trust.SourceRevision, Values: map[string]json.RawMessage{KeyAllow: json.RawMessage(rev)}})
		if err != nil {
			t.Fatal(err)
		}
		allow, _, notes := FromPolicy(eff)
		return allow, notes
	}
	if allow, notes := merge(`["team"]`); !slices.Equal(allow.IDs, []string{"team"}) || len(notes) != 0 {
		t.Fatalf("shrinking revision: %+v %v", allow, notes)
	}
	allow, notes := merge(`["team","mine","gpu"]`)
	if !slices.Equal(allow.IDs, []string{"mine", "team"}) || len(notes) != 1 {
		t.Fatalf("widening revision applied or unreported: %+v %v", allow, notes)
	}
	if _, _, err = reg.Merge("", trust.Scope{Kind: trust.SourceProtectedBranch, Values: map[string]json.RawMessage{KeyAllow: json.RawMessage(`["Bad"]`)}}); err == nil {
		t.Fatal("a malformed pool id was accepted")
	}
	eff, _, err := reg.Merge("", trust.Scope{Kind: trust.SourceRevision, Values: map[string]json.RawMessage{KeyDev: json.RawMessage(`["local"]`)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, dev, notes := FromPolicy(eff); dev.Set || len(notes) == 0 {
		t.Fatalf("pools.dev from a revision applied: %+v %v", dev, notes)
	}
}
