package main

// Purpose: tests of `nself ci runs decisions`, of the stored-record source of
// `nself ci explain <run>/<job>` and of the placement_ref that evidence carries
// (P7-SCHED-05). One real coordinator run under NSELF_V15=1 feeds all of them.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/schedtel"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const decisionSHA = "0123456789abcdef0123456789abcdef01234567"

func decisionGit(_ context.Context, argv ...string) (string, int, error) {
	j := strings.Join(argv, " ")
	switch {
	case strings.Contains(j, "rev-parse HEAD"):
		return decisionSHA + "\n", 0, nil
	case strings.Contains(j, "remote get-url"):
		return "git@github.com:acme/widgets.git\n", 0, nil
	case strings.Contains(j, "gh auth status"):
		return "ok\n", 0, nil
	}
	return "", 0, nil
}

type nopReporter struct{}

func (nopReporter) Post(context.Context, report.Status) error { return nil }

// dispatchedRun runs build -> test and lint through the dispatcher (NSELF_V15=1)
// and returns the run summary and the state home it wrote to.
func dispatchedRun(t *testing.T) (cirun.Result, string) {
	t.Helper()
	return dispatchedRunIn(t, t.TempDir(), t.TempDir())
}

func dispatchedRunIn(t *testing.T, root, home string) (cirun.Result, string) {
	t.Helper()
	t.Setenv("NSELF_V15", "1")
	yes := true
	sh := func(script string, deps ...string) model.Job {
		return model.Job{Kind: "test", Run: []string{"sh", "-c", script}, Deps: deps, Required: &yes}
	}
	pl, err := plan.Build(plan.Input{Config: model.PipelineConfig{Version: 1, Jobs: map[string]model.Job{"build": sh("echo build"), "test": sh("echo test", "build"), "lint": sh("echo lint")}},
		World: plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, CPU: 8, MemMB: 16384}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "f.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &cirun.Coordinator{Opts: cirun.Options{Root: root, Home: home}, Plan: pl, ConfigDigest: "cfg", Out: &bytes.Buffer{}, Capture: decisionGit, Reporter: nopReporter{},
		Capacity: func() ciexec.Capacity {
			return ciexec.Capacity{CPUs: 8, MemoryMB: 16384, FreeMemoryMB: 12000, BatteryPercent: -1, PluggedIn: true, Interactive: true}
		}, Poll: 10 * time.Millisecond}
	res, err := c.Run(context.Background())
	if err != nil || len(res.Summary.Jobs) != 3 {
		t.Fatalf("run: %v %+v", err, res.Summary.Jobs)
	}
	return res, home
}

func decisionData(t *testing.T, out string) []decisionView {
	t.Helper()
	var env struct {
		Data []decisionView `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("envelope: %v\n%s", err, out)
	}
	return env.Data
}

func TestDecisionListAndExplainFromARealRun(t *testing.T) {
	res, home := dispatchedRun(t)
	t.Setenv("NSELF_CI_HOME", home)
	planview.Register(schedtel.Source{Open: openDecisionStore})
	t.Cleanup(func() { planview.Register(nil) })

	code, out, errs := capture(t, func() int { return runRuns([]string{"decisions", res.RunID, "--json"}) })
	list := decisionData(t, out)
	if code != 0 || len(list) != 3 {
		t.Fatalf("exit %d, %d records, want one per placed job\n%s%s", code, len(list), out, errs)
	}
	for _, d := range list {
		if d.Outcome != "admitted" || d.Runner != "local" || d.Run != res.RunID || !strings.HasPrefix(d.Ref, "sched:decision/") || d.WorldDigest == "" {
			t.Errorf("record %+v", d)
		}
	}
	// a ref resolves, newest-first listing works, an unknown ref is an error
	code, out, _ = capture(t, func() int { return runRuns([]string{"decisions", list[0].Ref, "--json"}) })
	if one := decisionData(t, out); code != 0 || len(one) != 1 || one[0].Ref != list[0].Ref {
		t.Fatalf("ref lookup: %d %+v", code, one)
	}
	if code, _, _ = capture(t, func() int { return runRuns([]string{"decisions", "sched:decision/nope"}) }); code != 1 {
		t.Fatalf("unknown ref exit %d", code)
	}
	if code, out, _ = capture(t, func() int { return runRuns([]string{"decisions"}) }); code != 0 || !strings.Contains(out, "admitted") {
		t.Fatalf("human listing: %d\n%s", code, out)
	}

	resetPlanFlags()
	code, out, errs = capture(t, func() int { return runExplain([]string{res.RunID + "/build", "--json"}) })
	if code != 0 {
		t.Fatalf("explain exit %d\n%s%s", code, out, errs)
	}
	d, _ := explainEnv(t, out)
	if d.Source != planview.SourceDecisionRecord || len(d.Jobs) != 1 || d.Jobs[0].Name != "build" || d.Jobs[0].Placement == nil || d.Jobs[0].Placement.RunnerID != "local" || len(d.Jobs[0].Alternatives) == 0 {
		t.Fatalf("stored decision: %+v", d)
	}
}

func TestPlacementRefEvidenceCarriesTheDecisionRef(t *testing.T) {
	res, home := dispatchedRun(t)
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	row, ok, err := st.PipelineByID(context.Background(), res.RunID)
	if err != nil || !ok {
		t.Fatalf("pipeline row: %v %v", ok, err)
	}
	rows, err := st.EvidenceByRevision(context.Background(), decisionSHA, row.InputDigest)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, r := range rows {
		if r.AttemptID == "" {
			continue
		}
		seen++
		ev, err := attest.Verify(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if ev.PlacementRef == nil || *ev.PlacementRef != "sched:decision/"+r.AttemptID {
			t.Fatalf("attempt %s placement_ref = %v", r.AttemptID, ev.PlacementRef)
		}
		if d, found, err := st.TelDecision(context.Background(), r.AttemptID); err != nil || !found || d.Outcome != "admitted" {
			t.Fatalf("ref does not resolve to a stored decision: %+v %v %v", d, found, err)
		}
	}
	if seen != 3 {
		t.Fatalf("%d attempt documents, want 3", seen)
	}
	for _, j := range res.Summary.Jobs {
		if _, still := cirun.PlacementRef(context.Background(), j.AttemptID); still {
			t.Errorf("finalizer left the ref of %s", j.AttemptID)
		}
	}
}

// The first world of a run is built before its first decision; the history of
// earlier runs must already be in it.
func TestTelemetryFirstTickUsesStoredEstimates(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	first, _ := dispatchedRunIn(t, root, home)
	ctx := context.Background()
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	row, _, err := st.PipelineByID(ctx, first.RunID)
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range []string{"build", "test", "lint"} {
		for i := 0; i < 3; i++ { // three samples of 4321 ms: estimated
			o := store.TelOutcome{AttemptID: fmt.Sprintf("seed-%s-%d", job, i), Project: row.Project, JobKey: job, Runner: "local", RunnerClass: "local", Result: "passed", RuntimeMs: 4321, ColdStartMs: -1, TS: int64(i)}
			fold := func(cur store.TelEstimator, _ bool) store.TelEstimator {
				cur.RuntimeMs, cur.N = 4321, cur.N+1
				return cur
			}
			if err = st.TelRecordOutcome(ctx, o, fold); err != nil {
				t.Fatal(err)
			}
		}
	}
	second, _ := dispatchedRunIn(t, root, home)
	recs, err := st.TelDecisions(ctx, second.RunID, 0)
	if err != nil || len(recs) != 3 {
		t.Fatalf("decisions of the second run: %d %v", len(recs), err)
	}
	for _, d := range recs {
		var doc model.PlacementPlan
		if err = json.Unmarshal(d.Doc, &doc); err != nil {
			t.Fatal(err)
		}
		c := doc.Jobs[0].Placement.Components["runtime"]
		if c.ValueMs != 4321 || c.Confidence != "estimated" {
			t.Errorf("%s: runtime component %+v, want 4321 estimated on the first tick", d.JobName, c)
		}
	}
}
