package schedpolicy

import (
	"encoding/json"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func TestSchedPolicyFlagValues(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    Flags
		want map[string]string
	}{
		{"nothing, queued", Flags{}, nil},
		{"foreground default", Flags{Foreground: true}, map[string]string{KeyMaxQueue: `"10m0s"`}},
		{"explicit queue wins", Flags{Foreground: true, MaxQueue: "2m"}, map[string]string{KeyMaxQueue: `"2m"`}},
		{"queued run with a queue cap", Flags{MaxQueue: "90s"}, map[string]string{KeyMaxQueue: `"90s"`}},
		{"short preset name", Flags{Prefer: "local"}, map[string]string{KeyPreset: `"prefer-local"`}},
		{"full preset name", Flags{Prefer: "cheapest", Runner: "ms5"}, map[string]string{KeyPreset: `"cheapest"`, KeyRunner: `"ms5"`}},
	} {
		got, err := tc.f.Values()
		if err != nil || len(got) != len(tc.want) {
			t.Errorf("%s: %v %v", tc.name, got, err)
			continue
		}
		for k, v := range tc.want {
			if string(got[k]) != v {
				t.Errorf("%s: %s = %s want %s", tc.name, k, got[k], v)
			}
		}
	}
	for name, f := range map[string]Flags{
		"unknown preset":  {Prefer: "warp"},
		"empty-ish":       {Prefer: "prefer-"},
		"bad duration":    {MaxQueue: "soon"},
		"zero duration":   {MaxQueue: "0s"},
		"negative":        {MaxQueue: "-2m"},
		"bare number":     {MaxQueue: "600"},
		"bad with others": {Prefer: "local", MaxQueue: "x"},
	} {
		if _, err := f.Values(); err == nil || !strings.HasPrefix(err.Error(), "E691 /sched/") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if CheckRunner("") == nil || CheckRunner("ms5") != nil {
		t.Error("CheckRunner")
	}
}

// TestSchedPolicyForegroundQueueDefault: through the merge and Placement, a
// foreground run without --max-queue waits at most 10 minutes; a queued run
// has no cap; an operator cap below 10 minutes stays.
func TestSchedPolicyForegroundQueueDefault(t *testing.T) {
	run := func(f Flags, operator ...trust.Scope) int64 {
		t.Helper()
		vals, err := f.Values()
		if err != nil {
			t.Fatal(err)
		}
		scopes := append(operator, trust.Scope{Kind: trust.SourceRevision, Values: vals})
		eff, err := merge(t, scopes...)
		if err != nil {
			t.Fatal(err)
		}
		p, err := Placement(&eff)
		if err != nil {
			t.Fatal(err)
		}
		return p.Overrides.MaxQueueMs
	}
	if got := run(Flags{Foreground: true}); got != 600000 {
		t.Errorf("foreground default %d ms", got)
	}
	if got := run(Flags{}); got != 0 {
		t.Errorf("queued run capped at %d ms", got)
	}
	if got := run(Flags{Foreground: true, MaxQueue: "2m"}); got != 120000 {
		t.Errorf("--max-queue 2m gave %d ms", got)
	}
	if got := run(Flags{Foreground: true, MaxQueue: "3h"}); got != 10800000 {
		t.Errorf("a longer --max-queue gave %d ms", got)
	}
	cap5 := scope(trust.SourceProject, map[string]string{KeyMaxQueue: `"5m"`})
	if got := run(Flags{Foreground: true}, cap5); got != 300000 {
		t.Errorf("the revision loosened the operator cap to %d ms", got)
	}
}

// ---- --prefer through sched.Place on the D4 fixture world -----------------

type fixtureRow struct {
	ID         string `json:"id"`
	Class      string `json:"class"`
	Location   string `json:"location"`
	Authorized bool   `json:"authorized"`
	Wait       int64  `json:"wait_ms"`
	Runtime    int64  `json:"runtime_ms"`
	Cold       int64  `json:"cold_start_ms"`
	Transfer   int64  `json:"transfer_ms"`
	Contention int64  `json:"contention_ms"`
	Cost       int64  `json:"cost_micro"`
}

func ptr[T any](v T) *T { return &v }

func fixtureWorld(t *testing.T, p sched.Policy) sched.World {
	t.Helper()
	raw, err := os.ReadFile("testdata/placement/world.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows struct {
		Runners []fixtureRow `json:"runners"`
	}
	if err = json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	w := sched.World{Policy: p, Costs: map[string]sched.Cost{}, Previous: map[string]string{}, Estimates: map[string]map[string]sched.Sample{}}
	for _, row := range rows.Runners {
		c := model.Capability{Schema: "ci.runner-capability/v1"}
		c.Lifecycle.AuthorizedProjects = []string{"project"}
		if !row.Authorized {
			c.Lifecycle.AuthorizedProjects = []string{"someone-else"}
		}
		c.Lifecycle.Eligible = true
		c.Protocol.Versions, c.Protocol.Min = []int{1}, 1
		c.Platform.OS.Value, c.Platform.Arch.Value = ptr("linux"), ptr("amd64")
		c.Resources.CPU.Value, c.Resources.MemMB.Value = ptr(float64(8)), ptr(int64(8192))
		c.Identity.Labels = []string{"arch=amd64"}
		c.Location.Kind.Value = ptr(row.Location)
		w.Runners = append(w.Runners, sched.Runner{ID: row.ID, Location: row.Location, Class: row.Class, Provider: "operator", Capability: c,
			Avail: sched.Avail{Slots: 1, CPU: 8, MemMB: 8192},
			Decl:  trust.Declaration{Accepts: []model.TrustClass{"owner", "internal", "collaborator", "untrusted"}, Isolation: "vm", Network: "privileged", UIDSeparation: true, Ownership: "operator", PersonalOwnNode: row.Location == "local", NoLANRoute: true}})
		w.Estimates[row.ID] = map[string]sched.Sample{"j": {WaitMs: ptr(row.Wait), RuntimeMs: ptr(row.Runtime), ColdStartMs: ptr(row.Cold), TransferMs: ptr(row.Transfer),
			ContentionMs: ptr(row.Contention), CostMicro: ptr(row.Cost), ScarcityMs: ptr(int64(0)), ReliabilityMs: ptr(int64(0)), CacheMs: ptr(int64(0)), Confidence: "known"}}
	}
	return w
}

func fixtureJob() sched.Job {
	return sched.Job{Key: "j", ID: "j", Project: "project", Pipeline: sched.Pipeline{ID: "pipe", Support: []string{"linux/amd64"}},
		Requirements: model.PlacementRequirements{Platform: "linux/amd64", CPU: 1, MemMB: 128}, Trust: "owner", Network: "lan", TimeoutMs: 60000, PersonalOwnRun: true}
}

// TestSchedPolicyPreferGolden: `--prefer local` moves the placement from the
// LAN node to this machine, and no preset ever picks the runner that is not
// authorized for the project, however well it scores.
func TestSchedPolicyPreferGolden(t *testing.T) {
	got := map[string]string{}
	for _, flagValue := range []string{"", "local", "cheapest", "fastest", "balanced"} {
		values, err := Flags{Prefer: flagValue}.Values()
		if err != nil {
			t.Fatal(err)
		}
		// The preset is a preference, so it is applied as an operator scope here;
		// the revision scope would ignore it (trust.Merge: TightenAny).
		eff, err := merge(t, trust.Scope{Kind: trust.SourceProject, Values: values})
		if err != nil {
			t.Fatal(err)
		}
		p, err := Placement(&eff)
		if err != nil {
			t.Fatal(err)
		}
		res := sched.Place(fixtureWorld(t, p), []sched.Job{fixtureJob()})
		if len(res.Placements) != 1 {
			t.Fatalf("%q: %+v", flagValue, res)
		}
		name := flagValue
		if name == "" {
			name = "(default)"
		}
		got[name] = res.Placements[0].RunnerID
		if res.Placements[0].RunnerID == "X" {
			t.Errorf("%q placed the job on the unauthorized runner", flagValue)
		}
	}
	golden := "testdata/placement/prefer.golden.json"
	enc, _ := json.MarshalIndent(got, "", "  ")
	enc = append(enc, '\n')
	if *update {
		if err := os.WriteFile(golden, enc, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil || string(want) != string(enc) {
		t.Errorf("golden %s differs (run with -update):\nwant %s\ngot  %s (%v)", golden, want, enc, err)
	}
	if got["local"] != "L" || got["balanced"] != "N" || got["(default)"] != "N" {
		t.Errorf("--prefer local did not move the placement: %v", got)
	}
}
