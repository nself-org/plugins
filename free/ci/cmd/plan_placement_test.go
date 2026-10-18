package main

// Purpose: tests of `nself ci plan` placement (P7-SCHED-03): data.placement is
// a valid ci.placement-plan/v1 beside the unchanged plan keys, the store gets
// no row, no secret value is printed, and the human table has the column.
// Constraints: a real git repository from the planview fixture in a temp dir;
// the flag sets are package-level, so each call resets them.

import (
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
)

const fixtureSecret = "fixture-secret-value-0001"

// placementRepo copies the planview fixture into a git repository and points
// NSELF_CI_HOME and the secret-bearing environment at temp values.
func placementRepo(t *testing.T) (repo, home string) {
	t.Helper()
	repo, home = t.TempDir(), filepath.Join(t.TempDir(), "home")
	t.Setenv("NSELF_CI_HOME", home)
	t.Setenv("NSELF_CI_REPO", "")
	t.Setenv("FIXTURE_TOKEN", fixtureSecret)
	placementClock = func() time.Time { return time.Unix(1_700_000_000, 0) }
	t.Cleanup(func() { placementClock = time.Now })
	src := filepath.Join("..", "internal", "planview", "testdata", "repo")
	if err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		dst := filepath.Join(repo, rel)
		if err = os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "user.email=a@b", "-c", "user.name=t", "commit", "-qm", "init"}} {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return repo, home
}

func resetPlanFlags() {
	planOpts.json, planOpts.pipeline, planOpts.trigger, planOpts.repo, planOpts.cfg, planOpts.base = false, "", "", "", "", ""
	explainOpts.json, explainOpts.pipeline, explainOpts.trigger, explainOpts.repo, explainOpts.cfg, explainOpts.base = false, "", "", "", "", ""
	schedFlags = schedpolicy.Flags{}
}

func planJSON(t *testing.T, repo string, withPlacement bool) map[string]json.RawMessage {
	t.Helper()
	resetPlanFlags()
	saved := placePlan
	if !withPlacement {
		placePlan = nil
	}
	defer func() { placePlan = saved }()
	code, out, errs := capture(t, func() int { return runPlan([]string{"--json", "--repo", repo}) })
	if code != 0 {
		t.Fatalf("plan exit %d: %s %s", code, out, errs)
	}
	var env struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("envelope: %v\n%s", err, out)
	}
	env.Data["__raw"] = json.RawMessage(strings.TrimSpace(out))
	return env.Data
}

func TestPlanPlacementJSON(t *testing.T) {
	repo, _ := placementRepo(t)
	with, without := planJSON(t, repo, true), planJSON(t, repo, false)
	raw, ok := with["placement"]
	if !ok {
		t.Fatalf("no data.placement: %s", with["__raw"])
	}
	schema, err := os.ReadFile(filepath.Join("..", "schemas", "placement-plan.v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := model.ValidateJSON(schema, raw); err != nil {
		t.Fatalf("data.placement is not placement-plan v1: %v\n%s", err, raw)
	}
	for k := range without {
		if k == "__raw" {
			continue
		}
		if _, ok := with[k]; !ok {
			t.Errorf("plan key %q lost", k)
		}
	}
	var doc model.PlacementPlan
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Jobs) != 2 || doc.Jobs[0].ID != "build" || doc.Jobs[1].ID != "test" {
		t.Fatalf("jobs in plan order: %+v", doc.Jobs)
	}
	for _, j := range doc.Jobs {
		if j.Status != "placed" || j.Placement == nil || j.Placement.RunnerID != "local" {
			t.Errorf("%s: want placed on local, got %s %+v", j.ID, j.Status, j.Placement)
		}
	}
	if len(doc.Jobs[0].SecretClasses) != 1 || doc.Jobs[0].SecretClasses[0] != "project" {
		t.Errorf("secret classes: %v", doc.Jobs[0].SecretClasses)
	}
}

func TestPlanPlacementHuman(t *testing.T) {
	repo, _ := placementRepo(t)
	resetPlanFlags()
	code, out, errs := capture(t, func() int { return runPlan([]string{"--repo", repo}) })
	if code != 0 || !strings.Contains(out, "PLACEMENT") || strings.Count(out, "local (local, local) cost 0 micro [known]") != 2 {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
}

func TestPlanPlacementWritesNothing(t *testing.T) {
	repo, home := placementRepo(t)
	planJSON(t, repo, true) // creates state.db
	before := tableCounts(t, filepath.Join(home, "state.db"))
	if len(before) == 0 {
		t.Fatal("no tables read")
	}
	planJSON(t, repo, true)
	resetPlanFlags()
	capture(t, func() int { return runExplain([]string{"build", "--repo", repo}) })
	capture(t, func() int { return runExplain([]string{"run1/build", "--repo", repo}) })
	after := tableCounts(t, filepath.Join(home, "state.db"))
	for table, n := range before {
		if after[table] != n {
			t.Errorf("table %s: %d rows before, %d after", table, n, after[table])
		}
	}
}

func tableCounts(t *testing.T, path string) map[string]int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		names = append(names, n)
	}
	_ = rows.Close()
	out := map[string]int{}
	for _, n := range names {
		var c int
		if err := db.QueryRow(`SELECT COUNT(*) FROM "` + n + `"`).Scan(&c); err != nil {
			t.Fatal(err)
		}
		out[n] = c
	}
	return out
}

func TestPlanPlacementNoSecretValue(t *testing.T) {
	repo, _ := placementRepo(t)
	if os.Getenv("FIXTURE_TOKEN") != fixtureSecret { // positive control: the value is in the process
		t.Fatal("fixture secret not set")
	}
	for _, args := range [][]string{{"--repo", repo}, {"--json", "--repo", repo}} {
		resetPlanFlags()
		_, out, errs := capture(t, func() int { return runPlan(args) })
		if strings.Contains(out+errs, fixtureSecret) {
			t.Errorf("plan %v printed a secret value", args)
		}
	}
	for _, args := range [][]string{{"build", "--repo", repo}, {"build", "--json", "--repo", repo}} {
		resetPlanFlags()
		_, out, errs := capture(t, func() int { return runExplain(args) })
		if strings.Contains(out+errs, fixtureSecret) || !strings.Contains(out, "project") {
			t.Errorf("explain %v: secret value printed or class name missing\n%s", args, out)
		}
	}
}

func TestPlanPlacementCellsFollowPlanOrder(t *testing.T) {
	pl := &plan.Plan{Jobs: []plan.PlannedJob{{ID: "z"}, {ID: "a"}}}
	doc := model.PlacementPlan{Jobs: []model.JobPlacement{{ID: "a", Status: "waiting"}, {ID: "z", Status: "unplaceable"}}}
	got := cellsInPlanOrder(pl, doc)
	if len(got) != 2 || got[0] != "unplaceable" || got[1] != "waiting" {
		t.Fatalf("cells must follow the plan table, not the placement order: %v", got)
	}
}

func TestPlanPlacementRevisionUnknownWithoutGit(t *testing.T) {
	repo, _ := placementRepo(t)
	if err := os.RemoveAll(filepath.Join(repo, ".git")); err != nil {
		t.Fatal(err)
	}
	d := planJSON(t, repo, true)
	var doc model.PlacementPlan
	if err := json.Unmarshal(d["placement"], &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Pipeline.Revision != "" {
		t.Fatalf("git's message leaked into the revision: %q", doc.Pipeline.Revision)
	}
}

// writeConfig replaces the fixture's pipeline config in repo.
func writeConfig(t *testing.T, repo, yaml string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, ".nself", "ci.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
}

func placementOf(t *testing.T, repo string) model.PlacementPlan {
	t.Helper()
	d := planJSON(t, repo, true)
	var doc model.PlacementPlan
	if err := json.Unmarshal(d["placement"], &doc); err != nil {
		t.Fatalf("no placement: %v", err)
	}
	return doc
}

func TestPlanPlacementSkippedAndRefusedJobs(t *testing.T) {
	repo, _ := placementRepo(t)
	writeConfig(t, repo, `version: 1
jobs:
  build:
    kind: build
    run: [go, build, ./...]
  deploy:
    kind: release
    run: [echo, deploy]
    when:
      env_present: [P7_SCHED03_NOT_SET]
  svc:
    kind: test
    run: [echo, svc]
    requires: {tools: [no-such-tool-xyz]}
`)
	doc := placementOf(t, repo)
	byID := map[string]model.JobPlacement{}
	for _, j := range doc.Jobs {
		byID[j.ID] = j
	}
	if _, ok := byID["deploy"]; ok {
		t.Error("a skipped job needs no runner and must not be placed")
	}
	if b := byID["build"]; b.Status != "placed" {
		t.Errorf("build: %s", b.Status)
	}
	if doc.Totals.Placed != 1 {
		t.Errorf("totals count only what was placed: %+v", doc.Totals)
	}
	if s := byID["svc"]; s.Status != "unplaceable" || len(s.Reasons) != 1 || s.Reasons[0].Code != "unplaceable.tool" || s.Placement != nil {
		t.Errorf("the planner refused svc (tool missing); the scheduler must not place it: %+v", s)
	}
	resetPlanFlags()
	_, out, _ := capture(t, func() int { return runPlan([]string{"--repo", repo}) })
	if !strings.Contains(out, "skip: when.env_missing") {
		t.Errorf("the table says why the skip job has no runner:\n%s", out)
	}
	resetPlanFlags()
	code, out, _ := capture(t, func() int { return runExplain([]string{"deploy", "--json", "--repo", repo}) })
	if _, ec := explainEnv(t, out); code != 1 || ec != "E692" || !strings.Contains(out, "skipped by the plan") {
		t.Errorf("explain of a skip job: exit %d\n%s", code, out)
	}
}

func TestPlanPlacementStoreUnavailable(t *testing.T) {
	repo, _ := placementRepo(t)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NSELF_CI_HOME", filepath.Join(blocker, "home")) // a path under a regular file
	resetPlanFlags()
	code, out, errs := capture(t, func() int { return runPlan([]string{"--json", "--repo", repo}) })
	if code != 0 || !strings.Contains(errs, "placement unavailable") || strings.Contains(out, `"placement"`) || !strings.Contains(out, `"jobs"`) {
		t.Fatalf("the plan must survive without placement: exit %d\nstdout %s\nstderr %s", code, out, errs)
	}
}
