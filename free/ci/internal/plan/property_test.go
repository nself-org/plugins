package plan

// Property tests for the planner: 10,000 random cases (testing/quick) plus an
// exhaustive walk over a small enum space. Invariants: acyclic layers,
// determinism, every job reachable (planned or carrying a reason), required
// deep retention on default-branch and release plans against every selector
// and every branches/paths/triggers condition (Epic D28), and a skip check
// for a false env_present.

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

var (
	genBranches = []string{"main", "feature/x", "release/1", ""}
	genChanged  = []string{"a/x.go", "b/y.go", "c/z.go", "docs/r.md", "go.sum"}
	genPlats    = []string{"linux/amd64", "darwin/arm64"}
	genSelects  = []Selector{nil, noJobs, allJobs, someJobs}
)

func allJobs(in SelectionInput) (model.Selection, []string) {
	ids := make([]string, 0, len(in.Jobs))
	for _, r := range in.Jobs {
		ids = append(ids, r.ID)
	}
	return model.Selection{Mode: "affected", Reason: "selection.affected"}, ids
}

func someJobs(in SelectionInput) (model.Selection, []string) {
	var ids []string
	for i, r := range in.Jobs {
		if i%2 == 0 {
			ids = append(ids, r.ID)
		}
	}
	return model.Selection{Mode: "affected", Reason: "selection.affected"}, ids
}

type genCase struct {
	in  Input
	sel int
}

func pick[T any](r *rand.Rand, xs []T) T { return xs[r.Intn(len(xs))] }

func genWhen(r *rand.Rand) *model.WhenConfig {
	if r.Intn(2) == 0 {
		return nil
	}
	w := &model.WhenConfig{}
	if r.Intn(2) == 0 {
		w.Branches = []string{pick(r, []string{"main", "release/*", "never"})}
	}
	if r.Intn(2) == 0 {
		w.Paths = []string{pick(r, []string{"a/**", "docs", "nothing/**"})}
	}
	if r.Intn(2) == 0 {
		w.Triggers = []model.Trigger{pick(r, []model.Trigger{"push", "tag", "manual"})}
	}
	if r.Intn(3) == 0 {
		w.EnvPresent = []string{pick(r, []string{"TOKEN", "OTHER"})}
	}
	return w
}

func (genCase) Generate(r *rand.Rand, _ int) reflect.Value {
	n := 1 + r.Intn(7)
	cfg := model.PipelineConfig{Version: 1, Jobs: map[string]model.Job{}}
	for i := 0; i < n; i++ {
		j := model.Job{Kind: "test", Run: []string{"true"}, Workdir: pick(r, []string{".", "a", "b", "c"}), When: genWhen(r)}
		if r.Intn(3) > 0 {
			j.Required = bptr(r.Intn(3) > 0)
		}
		j.Path = pick(r, []model.JobPath{"", "fast", "deep"})
		j.AllowFailure = r.Intn(5) == 0
		if r.Intn(10) == 0 {
			j.Kind = "drill"
		}
		for k := 0; k < i; k++ {
			if r.Intn(3) == 0 {
				j.Deps = append(j.Deps, fmt.Sprintf("j%d", k))
			}
		}
		switch r.Intn(4) {
		case 0:
			j.Matrix = &model.JobMatrix{Platform: "support"}
		case 1:
			j.Matrix = &model.JobMatrix{Platform: []any{pick(r, genPlats)}}
		}
		if r.Intn(5) == 0 {
			j.Requirements = &model.Requirements{Tools: []string{pick(r, []string{"go", "xcode"})}}
		}
		cfg.Jobs[fmt.Sprintf("j%d", i)] = j
	}
	cfg.Support = append([]string(nil), genPlats[:1+r.Intn(2)]...)
	pipeline := ""
	if r.Intn(2) == 0 {
		listed := []string{fmt.Sprintf("j%d", r.Intn(n))}
		cfg.Pipelines = map[string]model.PipelineDef{"p": {Triggers: []model.Trigger{"push"}, Jobs: listed}}
		if r.Intn(2) == 0 {
			pipeline = "p"
		}
	}
	rev := Revision{Branch: pick(r, genBranches), DefaultBranch: r.Intn(2) == 0, Release: r.Intn(4) == 0, BaseKnown: r.Intn(4) > 0}
	for _, c := range genChanged {
		if r.Intn(3) == 0 {
			rev.Changed = append(rev.Changed, c)
		}
	}
	if r.Intn(2) == 0 {
		rev.Env = []string{"TOKEN"}
	}
	w := World{Platform: pick(r, genPlats), Docker: r.Intn(2) == 0, Tools: []string{"go"}, CPU: 4, MemMB: 4096}
	in := Input{Config: cfg, Revision: rev, World: w, Pipeline: pipeline, Trigger: pick(r, []model.Trigger{"", "push", "tag"})}
	return reflect.ValueOf(genCase{in: in, sel: r.Intn(len(genSelects))})
}

// invariants returns a description of the first violated invariant, or "".
func invariants(in Input) string {
	p, err := Build(in)
	expectE617 := false
	for id, j := range in.Config.Jobs {
		if effRequired(j) && reachesAllowFailure(in.Config.Jobs, id, map[string]bool{}) {
			expectE617 = true
		}
	}
	if expectE617 {
		if e, ok := err.(*Error); !ok || e.Code != CodeRequiredAllowFailure {
			return fmt.Sprintf("expected E617, got %v", err)
		}
		return ""
	}
	if err != nil {
		return fmt.Sprintf("unexpected error %v", err)
	}
	q, _ := Build(in)
	if string(mustJSON(p)) != string(mustJSON(q)) {
		return "not deterministic:\n" + string(mustJSON(p)) + "\n" + string(mustJSON(q))
	}
	layerOf := map[string]int{}
	listed := 0
	for _, l := range p.Layers {
		listed += len(l)
	}
	if listed != len(p.Jobs) {
		return "layers do not list every job once"
	}
	for _, j := range p.Jobs {
		layerOf[j.ID] = j.Layer
	}
	inPlan := map[string]bool{}
	for _, j := range p.Jobs {
		inPlan[j.Name] = true
		if !j.Reason.Valid() {
			return "job " + j.ID + " has no valid reason"
		}
		for _, d := range j.Deps {
			dl, ok := layerOf[d]
			if !ok || dl >= j.Layer {
				return "dep " + d + " of " + j.ID + " is missing or not earlier"
			}
		}
		if miss := envMissing(in.Config.Jobs[j.Name].When, in.Revision); len(miss) > 0 && j.State != StateSkip {
			return "job " + j.ID + " ran with a missing env_present"
		}
	}
	for _, s := range p.Skipped {
		if !s.Reason.Valid() {
			return "skipped " + s.Job + " has no valid reason"
		}
	}
	retain := in.Revision.DefaultBranch || in.Revision.Release
	for id, j := range in.Config.Jobs {
		if !inPlan[id] && skippedReason(p, id) == "" {
			return "job " + id + " is neither planned nor skipped with a reason"
		}
		if retain && effRequired(j) && effDeep(j) && j.Kind != "drill" && p.Pipeline.Kind != "drill" && !inPlan[id] {
			return "required deep job " + id + " dropped from a default-branch or release plan"
		}
	}
	return ""
}

// reachesAllowFailure reports a direct or transitive allow_failure dep.
func reachesAllowFailure(jobs map[string]model.Job, id string, seen map[string]bool) bool {
	for _, d := range jobs[id].Deps {
		if seen[d] {
			continue
		}
		seen[d] = true
		if jobs[d].AllowFailure || reachesAllowFailure(jobs, d, seen) {
			return true
		}
	}
	return false
}

func mustJSON(p *Plan) []byte {
	b, _ := json.Marshal(p)
	return b
}

func TestPlanProperties(t *testing.T) {
	defer SetSelector(nil)
	prop := func(c genCase) bool {
		SetSelector(genSelects[c.sel])
		if msg := invariants(c.in); msg != "" {
			t.Errorf("%s\nsel=%d rev=%+v support=%v world=%+v", msg, c.sel, c.in.Revision, c.in.Config.Support, c.in.World)
			return false
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 10000, Rand: rand.New(rand.NewSource(35))}); err != nil {
		t.Fatal(err)
	}
	cases := 0
	for bits := 0; bits < 8; bits++ {
		for ra := 0; ra < 2; ra++ {
			for rb := 0; rb < 2; rb++ {
				for pa := 0; pa < 2; pa++ {
					for pb := 0; pb < 2; pb++ {
						for w := 0; w < 6; w++ {
							for s := range genSelects {
								for pl := 0; pl < 3; pl++ {
									in := smallInput(bits, ra, rb, pa, pb, w, pl)
									SetSelector(genSelects[s])
									cases++
									if msg := invariants(in); msg != "" {
										t.Fatalf("%s (bits=%d ra=%d rb=%d pa=%d pb=%d when=%d sel=%d pipeline=%d)", msg, bits, ra, rb, pa, pb, w, s, pl)
									}
								}
							}
						}
					}
				}
			}
		}
	}
	if cases != 8*16*6*4*3 {
		t.Fatalf("walked %d cases", cases)
	}
}

// smallInput is the exhaustive space: two chained jobs, the second carrying
// one of six condition kinds (none, branch, paths, trigger, env, all three),
// planned as the default pipeline, the named pipeline q, or by trigger.
func smallInput(bits, ra, rb, pa, pb, when, pl int) Input {
	a := model.Job{Kind: "test", Run: []string{"true"}, Required: bptr(ra == 1), Path: []model.JobPath{"fast", "deep"}[pa]}
	b := model.Job{Kind: "test", Run: []string{"true"}, Required: bptr(rb == 1), Path: []model.JobPath{"fast", "deep"}[pb], Deps: []string{"a"}, Workdir: "a"}
	switch when {
	case 1:
		b.When = &model.WhenConfig{Branches: []string{"never"}}
	case 2:
		b.When = &model.WhenConfig{Paths: []string{"nothing/**"}}
	case 3:
		b.When = &model.WhenConfig{Triggers: []model.Trigger{"tag"}}
	case 4:
		b.When = &model.WhenConfig{EnvPresent: []string{"TOKEN"}}
	case 5:
		b.When = &model.WhenConfig{Branches: []string{"never"}, Paths: []string{"nothing/**"}, Triggers: []model.Trigger{"tag"}}
	}
	in := Input{
		Config: model.PipelineConfig{Version: 1, Jobs: map[string]model.Job{"a": a, "b": b},
			Pipelines: map[string]model.PipelineDef{"q": {Triggers: []model.Trigger{"push"}, Jobs: []string{"a"}}}},
		World:    linuxWorld(),
		Revision: Revision{Branch: "feature/x", DefaultBranch: bits&1 != 0, Release: bits&2 != 0, BaseKnown: bits&4 != 0, Changed: []string{"c/z.go"}},
	}
	switch pl {
	case 1:
		in.Pipeline = "q"
	case 2:
		in.Trigger = "push" // selects pipeline q
	}
	return in
}
