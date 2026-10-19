package planview

// Purpose: tests of the placement-plan document, the human lines, the job
// mapping, the decision source registry and the v1 schema fixtures.
// Constraints: no clock, no network, no disk outside testdata; goldens are
// rewritten with `go test ./internal/planview -run TestPlanviewGolden -update`.

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

var update = flag.Bool("update", false, "rewrite the golden files")

var fixedNow = time.Unix(1_700_000_000, 0)

// fixture returns a world with the local runner and the LAN Mac of the model
// fixtures, and three jobs: one placed, one waiting for the single slot and
// one outside the pipeline's support.
func fixture(t *testing.T) (sched.World, []sched.Job, sched.Result) {
	t.Helper()
	lc, err := LocalCapability(LocalFacts{Project: "proj", Platform: "linux/amd64", CPUs: 4, MemMB: 8192, Docker: true, Tools: []string{"go"}, Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	local := sched.Runner{ID: "local", Capability: lc, Decl: sched.DeclarationFromCapability(lc, false, ""), Class: "local", Location: "local", Provider: "local",
		Avail: sched.Avail{Slots: 1, CPU: 4, MemMB: 8192}}
	raw, err := os.ReadFile("../model/testdata/capability/valid/lan-mac.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var mc model.Capability
	if err = json.Unmarshal(raw, &mc); err != nil {
		t.Fatal(err)
	}
	mc.Identity.ID = "lan-mac"
	lan := sched.Runner{ID: "lan-mac", Capability: mc, Decl: sched.DeclarationFromCapability(mc, false, ""), Class: "owned", Location: "lan", Provider: "agent",
		Avail: sched.Avail{Slots: 1, CPU: 8, MemMB: 16384}}
	w := sched.World{NowMs: fixedNow.UnixMilli(), Runners: []sched.Runner{local, lan}, Policy: sched.Policy{Preset: "balanced"},
		Costs: map[string]sched.Cost{}, Previous: map[string]string{}}
	MarkPersonalLocal(&w)
	jobs, _ := SchedJobs(Context{PipelineID: "default", Project: "proj", Support: []string{"linux/amd64"}, DefaultPlatform: "linux/amd64", Foreground: true}, []PlannedJob{
		{ID: "test", Name: "test", Layer: 1, Deps: []string{"build"}, Job: model.Job{Kind: "test", Run: []string{"go", "test"}}},
		{ID: "build", Name: "build", Layer: 0, Job: model.Job{Kind: "build", Run: []string{"go", "build"}, Secrets: []string{"project/token", "project/other"}, Timeout: "10m"}},
		{ID: "ios", Name: "ios", Platform: "darwin/arm64", Layer: 1, Job: model.Job{Kind: "build", Run: []string{"xcodebuild"}}},
	})
	return w, jobs, sched.Place(w, jobs)
}

func buildDoc(t *testing.T) model.PlacementPlan {
	w, jobs, res := fixture(t)
	return Build(Input{Name: "default", Revision: strings.Repeat("a", 40), Trigger: "local", PolicyDigest: "p" + strings.Repeat("0", 63), WorldDigest: "w" + strings.Repeat("0", 63), World: w, Jobs: jobs, Result: res})
}

func TestPlanviewBuild(t *testing.T) {
	d := buildDoc(t)
	got := map[string]model.JobPlacement{}
	for _, j := range d.Jobs {
		got[j.ID] = j
	}
	if d.Schema != SchemaID || len(d.Jobs) != 3 || d.Pipeline.ID != nil || d.Pipeline.Preset != "balanced" {
		t.Fatalf("header: %+v", d)
	}
	if d.Jobs[0].ID != "build" || d.Jobs[1].ID != "ios" || d.Jobs[2].ID != "test" {
		t.Fatalf("plan order (layer, id): %v %v %v", d.Jobs[0].ID, d.Jobs[1].ID, d.Jobs[2].ID)
	}
	b := got["build"]
	if b.Status != "placed" || b.Placement == nil || b.Placement.RunnerID != "local" || b.Placement.Estimate.CostConfidence != "known" || *b.Placement.Estimate.CostMicro != 0 {
		t.Fatalf("build: %+v", b)
	}
	if b.Placement.Estimate.RuntimeMs != nil {
		t.Fatal("a stand-in runtime must be null, not a number")
	}
	if len(b.SecretClasses) != 1 || b.SecretClasses[0] != "project" {
		t.Fatalf("secret classes are names, de-duplicated: %v", b.SecretClasses)
	}
	if s := got["test"]; s.Status != "waiting" || s.Placement != nil || s.ExpectedWaitMs != nil {
		t.Fatalf("test: %+v", s)
	}
	if s := got["ios"]; s.Status != "unplaceable" || len(s.Reasons) == 0 || s.Reasons[0].Code != string(model.PlatformMismatch) {
		t.Fatalf("ios: %+v", s)
	}
	tot := d.Totals
	if tot.Placed != 1 || tot.Waiting != 1 || tot.Unplaceable != 1 || tot.AwaitingApproval != 0 || tot.CostConfidence != "known" || tot.EstimatedCostMicro != 0 {
		t.Fatalf("totals: %+v", tot)
	}
	if len(b.Alternatives) != 2 || b.Alternatives[0].RunnerID != "lan-mac" || b.Alternatives[1].RunnerID != "local" || b.Alternatives[0].Eligible || len(b.Alternatives[0].Reasons) == 0 {
		t.Fatalf("alternatives by runner id with reasons: %+v", b.Alternatives)
	}
}

func TestPlanviewNoNullArrays(t *testing.T) {
	raw, err := json.Marshal(buildDoc(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`"deps":null`, `"labels":null`, `"tools":null`, `"secret_classes":null`, `"reasons":null`, `"alternatives":null`, `"jobs":null`} {
		if strings.Contains(string(raw), bad) {
			t.Errorf("%s in document", bad)
		}
	}
}

func TestPlanviewDeterministic(t *testing.T) {
	a, _ := json.Marshal(buildDoc(t))
	b, _ := json.Marshal(buildDoc(t))
	if string(a) != string(b) {
		t.Fatal("two builds of one world differ")
	}
}

func TestPlanviewGolden(t *testing.T) {
	d := buildDoc(t)
	doc, err := json.MarshalIndent(Explanation{PlacementPlan: d, Source: SourcePlan}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	human := strings.Join(append(Cells(d), ExplainLines(d, SourcePlan)...), "\n") + "\n"
	for name, got := range map[string][]byte{"placement.golden.json": append(doc, '\n'), "explain.golden.txt": []byte(human)} {
		path := filepath.Join("testdata", name)
		if *update {
			if err := os.WriteFile(path, got, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(path)
		if err != nil || string(want) != string(got) {
			t.Errorf("%s differs from the golden (rerun with -update to review): %v\n%s", name, err, got)
		}
	}
}

func TestPlanviewSelect(t *testing.T) {
	d := buildDoc(t)
	one := Select(d, func(j model.JobPlacement) bool { return j.ID == "build" })
	if len(one.Jobs) != 1 || one.Totals.Placed != 1 || one.Totals.Waiting != 0 || one.Totals.CostConfidence != "known" {
		t.Fatalf("select: %+v", one.Totals)
	}
	none := Select(d, func(model.JobPlacement) bool { return false })
	if len(none.Jobs) != 0 || none.Totals.CostConfidence != "unknown" {
		t.Fatalf("empty select: %+v", none.Totals)
	}
}

func TestPlanviewSchemaFixtures(t *testing.T) {
	schema, err := os.ReadFile("../../schemas/placement-plan.v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, valid := range []bool{true, false} {
		dir := "invalid"
		if valid {
			dir = "valid"
		}
		paths, _ := filepath.Glob(filepath.Join("testdata", dir, "*.json"))
		if len(paths) == 0 {
			t.Fatalf("no %s fixtures", dir)
		}
		for _, p := range paths {
			body, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			err = model.ValidateJSON(schema, body)
			if valid && err != nil {
				t.Errorf("%s: %v", p, err)
			}
			if !valid && (err == nil || !strings.Contains(err.Error(), "/")) {
				t.Errorf("%s: want an error with a JSON pointer, got %v", p, err)
			}
		}
	}
	// Both the plan document and the explain wrapper are valid v1.
	for _, v := range []any{buildDoc(t), Explanation{PlacementPlan: buildDoc(t), Source: SourcePlan}} {
		raw, _ := json.Marshal(v)
		if err := model.ValidateJSON(schema, raw); err != nil {
			t.Errorf("built document: %v", err)
		}
	}
}

func TestPlanviewSchemaRejectsAfterBuild(t *testing.T) {
	schema, _ := os.ReadFile("../../schemas/placement-plan.v1.schema.json")
	d := buildDoc(t)
	neg := int64(-1)
	d.Jobs[0].Placement.Estimate.CostMicro = &neg
	raw, _ := json.Marshal(d)
	if err := model.ValidateJSON(schema, raw); err == nil || !strings.Contains(err.Error(), "cost_micro") {
		t.Fatalf("negative cost_micro: %v", err)
	}
	d = buildDoc(t)
	d.Jobs[0].Status, d.Jobs[0].Placement = "ineligible", nil
	raw, _ = json.Marshal(d)
	if err := model.ValidateJSON(schema, raw); err == nil || !strings.Contains(err.Error(), "status") {
		t.Fatalf("unknown status: %v", err)
	}
}

func TestPlanviewSource(t *testing.T) {
	Register(nil)
	_, err := Lookup(context.Background(), "run1", "build")
	var pe *Error
	if !asError(err, &pe) || pe.Code != "E692" || !strings.Contains(pe.Message, "nself ci runs decisions") {
		t.Fatalf("no source: %v", err)
	}
	Register(stubSource{})
	defer Register(nil)
	d, err := Lookup(context.Background(), "run1", "build")
	if err != nil || d.Pipeline.Name != "from-source" {
		t.Fatalf("registered source: %+v %v", d, err)
	}
}

type stubSource struct{}

func (stubSource) Decision(context.Context, string, string) (model.PlacementPlan, error) {
	return model.PlacementPlan{Schema: SchemaID, Pipeline: model.PlacementPipelineRef{Name: "from-source"}, Jobs: []model.JobPlacement{{ID: "build", Status: "waiting"}}}, nil
}

func asError(err error, target **Error) bool {
	e, ok := err.(*Error)
	*target = e
	return ok
}

func TestPlanviewJobs(t *testing.T) {
	jobs, _ := SchedJobs(Context{PipelineID: "p", Project: "x", DefaultPlatform: "linux/amd64"}, []PlannedJob{
		{ID: "b", Name: "b", Layer: 1, Job: model.Job{Isolation: "container", Secrets: []string{"deploy/a", "release/b", "none/c", "bogus/d"}, Timeout: "bad"}},
		{ID: "a", Name: "a", Layer: 1, Platform: "darwin/arm64", Job: model.Job{Kind: "release", Requirements: &model.Requirements{Tools: []string{"xcode"}, CPU: 2}}},
		{ID: "z", Name: "z", Layer: 0},
	})
	if jobs[0].ID != "z" || jobs[1].ID != "a" || jobs[2].ID != "b" {
		t.Fatalf("order: %s %s %s", jobs[0].ID, jobs[1].ID, jobs[2].ID)
	}
	if got := jobs[1].Pipeline.Support; len(got) != 2 || got[0] != "darwin/arm64" || got[1] != "linux/amd64" {
		t.Fatalf("support defaults to the platforms the jobs use: %v", got)
	}
	b := jobs[2]
	if b.TimeoutMs != DefaultTimeoutMs || len(b.Requirements.Tools) != 1 || b.Requirements.Tools[0] != "docker" || b.Pipeline.Band != "normal" {
		t.Fatalf("defaults: %+v", b)
	}
	if len(b.SecretClasses) != 2 || b.SecretClasses[0] != "deploy" || b.SecretClasses[1] != "release" {
		t.Fatalf("classes keep known names only: %v", b.SecretClasses)
	}
	if !jobs[1].Pipeline.Release || jobs[1].Requirements.CPU != 2 || jobs[1].MatrixKey != "darwin/arm64" {
		t.Fatalf("release job: %+v", jobs[1])
	}
}

func TestPlanviewLocalCapability(t *testing.T) {
	c, err := LocalCapability(LocalFacts{Project: "p", Platform: "linux/amd64", CPUs: 1, MemMB: 1024, Now: fixedNow})
	if err != nil {
		t.Fatalf("local capability must be a valid ci.runner-capability/v1: %v", err)
	}
	if err := model.ValidateCapability(c); err != nil {
		t.Fatal(err)
	}
}

func i64(v int64) *int64 { return &v }

func TestPlanviewTotalsWorstConfidence(t *testing.T) {
	sum, conf := total([]costFact{{i64(5), "known"}, {i64(7), "estimated"}, {i64(1), "computed"}})
	if sum != 13 || conf != "estimated" {
		t.Fatalf("sum %d confidence %s", sum, conf)
	}
	if sum, conf = total([]costFact{{i64(5), "known"}, {nil, "unknown"}}); sum != 5 || conf != "unknown" {
		t.Fatalf("an unknown cost makes the total unknown: %d %s", sum, conf)
	}
	if _, conf = total(nil); conf != "unknown" {
		t.Fatal("nothing placed: nothing is known")
	}
}

func TestPlanviewCostConfidence(t *testing.T) {
	w := sched.World{Costs: map[string]sched.Cost{"h": {Confidence: "computed"}}}
	for _, c := range []struct {
		r    sched.Runner
		want string
	}{
		{sched.Runner{ID: "a", Class: "local"}, "known"}, {sched.Runner{ID: "a", Class: "owned"}, "known"},
		{sched.Runner{ID: "m", Class: "metered"}, "unknown"}, {sched.Runner{ID: "h", Class: "metered"}, "computed"},
	} {
		if got := costConfidence(w, c.r); got != c.want {
			t.Errorf("%s/%s: %s, want %s", c.r.ID, c.r.Class, got, c.want)
		}
	}
	x := sched.Explanation{Alternatives: []model.Alternative{{RunnerID: "m", Eligible: true, Score: i64(1), Components: map[string]model.Component{}}}}
	pl, cf := placement(w, sched.Runner{ID: "m", Class: "metered"}, sched.Placement{RunnerID: "m", CostMicro: 99}, x)
	if pl.Estimate.CostMicro != nil || cf.micro != nil || pl.Estimate.CostConfidence != "unknown" {
		t.Fatalf("an unknown cost is null, not 99: %+v", pl.Estimate)
	}
}

func TestPlanviewMarkPersonalLocal(t *testing.T) {
	w := sched.World{Runners: []sched.Runner{{ID: "local"}, {ID: "other"}, {ID: "local", CoordinatorHost: true, CoordinatorMode: "team"}}}
	MarkPersonalLocal(&w)
	if !w.Runners[0].Decl.PersonalOwnNode || w.Runners[0].CoordinatorMode != "personal" {
		t.Fatalf("local runner: %+v", w.Runners[0])
	}
	if w.Runners[1].Decl.PersonalOwnNode || w.Runners[2].Decl.PersonalOwnNode || w.Runners[2].CoordinatorMode != "team" {
		t.Fatal("only a runner called local that does not host a TEAM coordinator is marked")
	}
	w = sched.World{Runners: []sched.Runner{{ID: "local", CoordinatorHost: true, CoordinatorMode: "personal"}}}
	MarkPersonalLocal(&w)
	if !w.Runners[0].Decl.PersonalOwnNode {
		t.Fatal("the node of a PERSONAL coordinator is the operator's own node")
	}
}

func TestPlanviewSelectCounts(t *testing.T) {
	d := buildDoc(t)
	all := Select(d, func(model.JobPlacement) bool { return true })
	if all.Totals != d.Totals {
		t.Fatalf("select all changed the totals: %+v vs %+v", all.Totals, d.Totals)
	}
	ios := Select(d, func(j model.JobPlacement) bool { return j.ID == "ios" })
	if ios.Totals.Unplaceable != 1 || ios.Totals.Waiting != 0 || ios.Totals.Placed != 0 {
		t.Fatalf("unplaceable counted as %+v", ios.Totals)
	}
}

func TestPlanviewHumanUnknownComponent(t *testing.T) {
	lines := strings.Join(ExplainLines(buildDoc(t), SourcePlan), "\n")
	if !strings.Contains(lines, "runtime=unknown(stand-in 300000)") || strings.Contains(lines, "runtime=300000") {
		t.Fatal("a stand-in runtime must not read as a measurement")
	}
}

func TestPlanviewBlockedAndTools(t *testing.T) {
	run, blocked := SchedJobs(Context{PipelineID: "p", Project: "x", DefaultPlatform: "linux/amd64"}, []PlannedJob{
		{ID: "ok", Name: "ok", Tool: "gitleaks", Job: model.Job{NeedsServices: []string{"postgres"}}},
		{ID: "skipped", Name: "skipped", State: "skip", Blocked: "when.env_missing", Detail: "environment not set: X"},
		{ID: "stuck", Name: "stuck", State: "unplaceable", Blocked: "unplaceable.tool", Detail: "tool x is not on PATH"},
	})
	if len(run) != 1 || run[0].ID != "ok" || len(blocked) != 1 || blocked[0].Job.ID != "stuck" || blocked[0].Code != "unplaceable.tool" {
		t.Fatalf("a skipped job needs no runner and the planner's unplaceable job is not placed: %v %v", run, blocked)
	}
	if got := run[0].Requirements.Tools; len(got) != 2 || got[0] != "docker" || got[1] != "gitleaks" {
		t.Fatalf("services need docker and a catalogue check needs its tool: %v", got)
	}
	w, jobs, res := fixture(t)
	d := Build(Input{Name: "n", Trigger: "local", PolicyDigest: "p", WorldDigest: "w", World: w, Jobs: jobs, Result: res, Blocked: blocked})
	last := d.Jobs[len(d.Jobs)-1]
	if last.ID != "stuck" || last.Status != "unplaceable" || last.Reasons[0].Code != "unplaceable.tool" || d.Totals.Unplaceable != 2 {
		t.Fatalf("blocked job: %+v totals %+v", last, d.Totals)
	}
}

type fixedSource struct{ doc model.PlacementPlan }

func (f fixedSource) Decision(context.Context, string, string) (model.PlacementPlan, error) {
	return f.doc, nil
}

func TestPlanviewLookupValidates(t *testing.T) {
	defer Register(nil)
	good := buildDoc(t)
	for name, doc := range map[string]model.PlacementPlan{
		"empty":     {},
		"nil jobs":  {Schema: SchemaID},
		"other job": {Schema: SchemaID, Jobs: []model.JobPlacement{{ID: "zzz"}}},
	} {
		Register(fixedSource{doc})
		var pe *Error
		if _, err := Lookup(context.Background(), "run1", "build"); !asError(err, &pe) || pe.Code != "E692" {
			t.Errorf("%s: want E692, got %v", name, err)
		}
	}
	Register(fixedSource{good})
	d, err := Lookup(context.Background(), "run1", "build")
	if err != nil || len(d.Jobs) != 1 || d.Jobs[0].ID != "build" || d.Totals.Placed != 1 {
		t.Fatalf("the asked job alone, with its own totals: %+v %v", d.Totals, err)
	}
}

func TestPlanviewLookupMatrixName(t *testing.T) {
	defer Register(nil)
	doc := model.PlacementPlan{Schema: SchemaID, Jobs: []model.JobPlacement{{ID: "build@linux/amd64", Name: "build", Status: "waiting"}, {ID: "build@darwin/arm64", Name: "build", Status: "waiting"}}}
	Register(fixedSource{doc})
	var pe *Error
	if _, err := Lookup(context.Background(), "run", "build"); !asError(err, &pe) || pe.Code != "E692" || !strings.Contains(pe.Message, "matrix") {
		t.Fatalf("a bare name for two matrix entries is ambiguous: %v", err)
	}
	d, err := Lookup(context.Background(), "run", "build@darwin/arm64")
	if err != nil || len(d.Jobs) != 1 || d.Jobs[0].ID != "build@darwin/arm64" {
		t.Fatalf("an exact id: %+v %v", d.Jobs, err)
	}
}

// TestPlanviewSchemaDocumentsTotalsCost pins D-0992: estimated_cost_micro is
// the sum of the known placed costs (0 when none is known), so the schema must
// tell consumers to read cost_confidence first.
func TestPlanviewSchemaDocumentsTotalsCost(t *testing.T) {
	raw, err := os.ReadFile("../../schemas/placement-plan.v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Properties struct {
			Totals struct {
				Properties map[string]struct {
					Description string `json:"description"`
				} `json:"properties"`
			} `json:"totals"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	cost := s.Properties.Totals.Properties["estimated_cost_micro"].Description
	for _, want := range []string{"sum", "cost_confidence", "unknown"} {
		if !strings.Contains(cost, want) {
			t.Errorf("totals.estimated_cost_micro description %q lacks %q", cost, want)
		}
	}
	if s.Properties.Totals.Properties["cost_confidence"].Description == "" {
		t.Error("totals.cost_confidence has no description")
	}
}
