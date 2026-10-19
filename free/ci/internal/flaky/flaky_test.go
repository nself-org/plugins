package flaky

// A check that alternates pass/fail on identical inputs across three real runs
// is listed with its runs, and the listing changes no result.

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/run"
)

const testSHA = "0123456789abcdef0123456789abcdef01234567"

type okReporter struct{}

func (okReporter) Post(context.Context, report.Status) error { return nil }

func fakeGit(rev string) func(context.Context, ...string) (string, int, error) {
	return func(_ context.Context, argv ...string) (string, int, error) {
		switch j := strings.Join(argv, " "); {
		case strings.Contains(j, "rev-parse HEAD"):
			return rev + "\n", 0, nil
		case strings.Contains(j, "remote get-url"):
			return "git@github.com:acme/widgets.git\n", 0, nil
		}
		return "", 0, nil
	}
}

// runJob runs one job named "unit" once at rev in root and returns its run id.
func runJob(t *testing.T, root, home, rev, script string) run.Result {
	t.Helper()
	yes := true
	pl, err := plan.Build(plan.Input{Config: model.PipelineConfig{Version: 1, Jobs: map[string]model.Job{
		"unit": {Kind: "test", Run: []string{"sh", "-c", script}, Required: &yes}}},
		World: plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, CPU: 8, MemMB: 16384}})
	if err != nil {
		t.Fatal(err)
	}
	c := &run.Coordinator{Opts: run.Options{Root: root, Home: home}, Plan: pl, ConfigDigest: "cfg", Out: io.Discard, Capture: fakeGit(rev),
		Reporter: okReporter{}, Poll: 10 * time.Millisecond,
		Capacity: func() ciexec.Capacity {
			return ciexec.Capacity{CPUs: 8, MemoryMB: 16384, FreeMemoryMB: 12000, BatteryPercent: -1, PluggedIn: true, Interactive: true}
		}}
	res, err := c.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestFlakyListing(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	marker := filepath.Join(t.TempDir(), "runs")
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// odd runs pass, even runs fail: the same job on the same revision and inputs
	script := `n=$(wc -l < ` + marker + `); echo x >> ` + marker + `; if [ $((n % 2)) -eq 1 ]; then echo "--- FAIL: TestAlternates"; exit 1; fi`
	var ids []string
	var results []model.Result
	for i := 0; i < 3; i++ {
		res := runJob(t, root, home, testSHA, script)
		ids, results = append(ids, res.Summary.RunID), append(results, res.Summary.Result)
	}
	// a stable check on another revision must not appear
	runJob(t, root, home, "fedcba9876543210fedcba9876543210fedcba98", "exit 0")
	st, err := run.OpenStore(filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	got, err := Scan(context.Background(), st, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Check != "unit" || got[0].Source != SourceHistory || got[0].Passes != 2 || got[0].Fails != 1 || got[0].Revision != testSHA {
		t.Fatalf("listing %+v", got)
	}
	listed := map[string]model.CheckResult{}
	for _, r := range got[0].Runs {
		listed[r.Run] = r.Result
	}
	for i, id := range ids {
		want := model.CheckResult("pass")
		if results[i] != "pass" {
			want = "fail"
		}
		if listed[id] != want {
			t.Errorf("run %s listed as %q, ran as %q", id, listed[id], want)
		}
	}
	// the listing is read-only: scanning again gives the same answer and the failed run is still a fail
	again, _ := Scan(context.Background(), st, 0)
	if len(again) != 1 || again[0].Passes != 2 || again[0].Fails != 1 {
		t.Fatalf("second scan %+v", again)
	}
	fails := 0
	for _, r := range results {
		if r == "fail" {
			fails++
		}
	}
	if fails != 1 {
		t.Fatalf("results %v: the listing or the retries changed a result", results)
	}
}

func evidenceOf(rev, tracked, runID string, results ...model.CheckResult) Doc {
	ev := model.Evidence{Revision: rev, RepoID: "repo", Pipeline: model.PipelineRef{Name: "default", Kind: "default"},
		InputDigests: model.InputDigests{Config: "cfg"}}
	if tracked != "" {
		ev.TrackedChangesDigest = &tracked
	}
	for i, r := range results {
		ev.Checks = append(ev.Checks, model.Check{ID: []string{"unit", "lint"}[i], JobID: []string{"unit", "lint"}[i], Result: r})
	}
	return Doc{RunID: runID, Evidence: ev}
}

func TestDetectNeedsIdenticalInputs(t *testing.T) {
	for name, tc := range map[string]struct {
		docs []Doc
		want int
	}{
		"same inputs, differing result":       {[]Doc{evidenceOf("r1", "", "a", "pass"), evidenceOf("r1", "", "b", "fail")}, 1},
		"different revisions are not a flake": {[]Doc{evidenceOf("r1", "", "a", "pass"), evidenceOf("r2", "", "b", "fail")}, 0},
		"different dirty trees are not":       {[]Doc{evidenceOf("r1", "d1", "a", "pass"), evidenceOf("r1", "d2", "b", "fail")}, 0},
		"an infra error says nothing":         {[]Doc{evidenceOf("r1", "", "a", "pass"), evidenceOf("r1", "", "b", "error")}, 0},
		"a skip says nothing":                 {[]Doc{evidenceOf("r1", "", "a", "pass"), evidenceOf("r1", "", "b", "skip")}, 0},
		"always failing is not flaky":         {[]Doc{evidenceOf("r1", "", "a", "fail"), evidenceOf("r1", "", "b", "fail")}, 0},
		"one run twice counts once":           {[]Doc{evidenceOf("r1", "", "a", "pass"), evidenceOf("r1", "", "a", "fail")}, 0},
	} {
		if got := Detect(tc.docs); len(got) != tc.want {
			t.Errorf("%s: %d flakes, want %d: %+v", name, len(got), tc.want, got)
		}
	}
}

func TestDetectKeepsChecksApart(t *testing.T) {
	got := Detect([]Doc{evidenceOf("r1", "", "a", "pass", "pass"), evidenceOf("r1", "", "b", "fail", "pass")})
	if len(got) != 1 || got[0].Check != "unit" {
		t.Fatalf("only unit differs: %+v", got)
	}
}

func TestDetectListsConfirmationFlakes(t *testing.T) {
	d := evidenceOf("r1", "", "a", "fail")
	d.Evidence.Checks[0].Flaky = &model.Flaky{Source: "confirmation", Passes: 1, Fails: 2}
	got := Detect([]Doc{d})
	if len(got) != 1 || got[0].Source != SourceConfirmation || got[0].Passes != 1 || got[0].Fails != 2 || got[0].Runs[0].Result != "fail" {
		t.Fatalf("listing %+v", got)
	}
	adhoc := d
	adhoc.Evidence.Pipeline.Kind = "adhoc"
	if got := Detect([]Doc{adhoc}); len(got) != 0 {
		t.Fatalf("an adhoc run was listed: %+v", got)
	}
}
