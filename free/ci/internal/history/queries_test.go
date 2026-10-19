package history

// `ci runs list` over real runs: result, duration, binding and freshness come
// from the run's evidence, and a run with no evidence reads unknown.

import (
	"context"
	"io"
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

func fakeGit(_ context.Context, argv ...string) (string, int, error) {
	switch j := strings.Join(argv, " "); {
	case strings.Contains(j, "rev-parse HEAD"):
		return testSHA + "\n", 0, nil
	case strings.Contains(j, "remote get-url"):
		return "git@github.com:acme/widgets.git\n", 0, nil
	}
	return "", 0, nil
}

func runOnce(t *testing.T, home, script string) run.Result {
	t.Helper()
	yes := true
	pl, err := plan.Build(plan.Input{Config: model.PipelineConfig{Version: 1, Jobs: map[string]model.Job{
		"a": {Kind: "test", Run: []string{"sh", "-c", script}, Required: &yes}}},
		World: plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, CPU: 8, MemMB: 16384}})
	if err != nil {
		t.Fatal(err)
	}
	c := &run.Coordinator{Opts: run.Options{Root: t.TempDir(), Home: home}, Plan: pl, ConfigDigest: "cfg", Out: io.Discard, Capture: fakeGit,
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

func TestListShowsResultDurationBindingAndLogs(t *testing.T) {
	home := t.TempDir()
	pass := runOnce(t, home, "echo hello; echo world")
	fail := runOnce(t, home, "echo broken; exit 3")
	st, err := run.OpenStore(home + "/state.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	es, err := List(context.Background(), Env{Store: st, Home: home}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 2 || es[0].ID != fail.Summary.RunID || es[1].ID != pass.Summary.RunID {
		t.Fatalf("entries %+v", es)
	}
	if es[0].Result != "fail" || es[1].Result != "pass" {
		t.Errorf("results %q %q", es[0].Result, es[1].Result)
	}
	for _, e := range es {
		if e.DurationMS == nil || e.Revision != testSHA || e.Binding == "" || e.Binding == "unknown" || e.LogBytes == 0 || e.StartedAt == "" {
			t.Errorf("entry incomplete: %+v", e)
		}
		if e.Freshness != "unknown" || e.FreshnessReason == nil {
			t.Errorf("with no repository the freshness must be unknown with a reason: %+v", e)
		}
	}
	if one, _ := List(context.Background(), Env{Store: st, Home: home}, 1); len(one) != 1 || one[0].ID != fail.Summary.RunID {
		t.Fatalf("limit 1: %+v", one)
	}
}

func TestListWithoutAStoreOrEvidenceNeverReadsAsAPass(t *testing.T) {
	if es, err := List(context.Background(), Env{}, 5); err != nil || es != nil {
		t.Fatalf("no store: %v %v", es, err)
	}
	f := newFixture(t)
	f.add("bare", "passed", time.Hour, 0)
	es, err := List(context.Background(), Env{Store: f.st, Home: f.home}, 5)
	if err != nil || len(es) != 1 {
		t.Fatalf("%v %v", es, err)
	}
	if e := es[0]; e.Result != "unknown" || e.Binding != "unknown" || e.Freshness != "unknown" || e.Status != "passed" || e.DurationMS == nil {
		t.Fatalf("a run with no evidence: %+v", e)
	}
}
