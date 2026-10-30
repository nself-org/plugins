package where

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

var world = Known{Nodes: []string{"ms5", "lan1"}, Pools: []string{"lan", "gpu"}, Providers: []string{"github", "gitlab"}}

func TestWhereMappingTable(t *testing.T) {
	for _, tc := range []struct {
		flag string
		k    Known
		o    Opts
		kind string
		want map[string]string
	}{
		{"local", world, Opts{}, KindLocal, map[string]string{"sched.overrides.local_only": `true`}},
		{"ms5", world, Opts{}, KindNode, map[string]string{"sched.overrides.runner": `"ms5"`}},
		{"lan", world, Opts{}, KindPool, map[string]string{"pools.allow": `["lan"]`}},
		{"lan", world, Opts{Dev: true}, KindPool, map[string]string{"pools.dev": `["lan"]`}},
		{"hosted", world, Opts{}, KindHosted, map[string]string{"sched.overrides.providers.allow": `["github","gitlab"]`}},
		{"auto", world, Opts{}, KindAuto, nil},
		{"all", world, Opts{RaceReady: true}, KindAll, nil},
	} {
		got, err := Values(tc.flag, tc.k, tc.o)
		if err != nil || got.Kind != tc.kind || len(got.Values) != len(tc.want) {
			t.Errorf("%s: %+v %v", tc.flag, got, err)
			continue
		}
		for key, v := range tc.want {
			if string(got.Values[key]) != v {
				t.Errorf("%s: %s = %s want %s", tc.flag, key, got.Values[key], v)
			}
		}
	}
}

func TestWhereRefusals(t *testing.T) {
	for name, tc := range map[string]struct {
		flag, sub string
		k         Known
	}{
		"unknown":       {"nowhere", "unknown value", world},
		"empty":         {"", "needs a value", world},
		"no providers":  {"hosted", "no hosted provider", Known{}},
		"pool excluded": {"gpu", "pool.excluded", Known{Pools: []string{"gpu", "lan"}, Effective: []string{"lan"}}},
		"node excluded": {"ms5", "pool.excluded", Known{Nodes: []string{"ms5"}, Excluded: map[string]string{"ms5": "pool lan only"}}},
	} {
		_, err := Values(tc.flag, tc.k, Opts{})
		if err == nil || !strings.HasPrefix(err.Error(), CodeInvalid) || !strings.Contains(err.Error(), tc.sub) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestWhereAllNotYet(t *testing.T) {
	_, err := Values("all", world, Opts{})
	if err == nil || err.Error() != "E797: race not available" {
		t.Fatalf("got %v", err)
	}
}

// TestWhereProjectFixedWins: a project-fixed runner beats `--where <other>`;
// the revision value is ignored and reported by the merge.
func TestWhereProjectFixedWins(t *testing.T) {
	tgt, err := Values("ms5", world, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	fixed, _ := json.Marshal("lan1")
	reg, err := trust.NewRegistry(policy.Default.Keys()...)
	if err != nil {
		t.Fatal(err)
	}
	eff, _, err := reg.Merge("",
		trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{"sched.overrides.runner": fixed}},
		trust.Scope{Kind: trust.SourceRun, Values: tgt.Values})
	if err != nil {
		t.Fatal(err)
	}
	v, _, _ := eff.Value("sched.overrides.runner")
	if v != "lan1" {
		t.Fatalf("runner = %v, want the project's lan1", v)
	}
	if len(eff.Ignored()) == 0 {
		t.Error("the ignored revision value must be reported")
	}
}
