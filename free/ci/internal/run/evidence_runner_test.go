package run

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func remoteRunner(node string) model.AttemptRunner {
	return model.AttemptRunner{Kind: "ssh", Tier: "lan", Pool: "", Rank: 0, Node: node, Provider: "ssh", Platform: "linux/arm64", Isolation: "container"}
}

// admit plays the run admitting attempt n of job on node; a non-empty detail is what the dispatcher noted of it.
func admit(t *testing.T, runID, job string, n int, node string, local bool, detail *model.AttemptRunner) JobOutcome {
	t.Helper()
	id := runID + "." + job + ".a" + string(rune('0'+n))
	NoteAdmission(runID, job, n, Admitted{AttemptID: id, RunnerID: node, Local: local})
	if detail != nil {
		NoteRunner(id, *detail)
	}
	t.Cleanup(func() { ForgetRun(runID) })
	return JobOutcome{ID: job, AttemptID: id, N: n, State: StatePassed, Required: true}
}

func runnerEvidence(t *testing.T, runID string, jobs []JobOutcome) model.Evidence {
	t.Helper()
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	ev, _, err := buildEvidence(evidenceInput{RunID: runID, Revision: strings.Repeat("a", 40), Start: start, End: start, Trigger: "local", Jobs: jobs,
		Pipeline:  model.PipelineRef{ID: "default", Name: "default", Kind: "default"},
		Selection: model.Selection{Mode: "full", Reason: "selection.no_base"}})
	if err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile(filepath.Join("..", "..", "schemas", "ci-evidence.v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := json.Marshal(ev)
	if err := model.ValidateJSON(schema, doc); err != nil {
		t.Fatalf("document does not validate: %v\n%s", err, doc)
	}
	return ev
}

func nodes(ev model.Evidence) string {
	var out []string
	for _, a := range ev.Attempts {
		out = append(out, a.JobID+"#"+string(rune('0'+a.Attempt))+"@"+a.Runner.Node)
	}
	return strings.Join(out, " ")
}

// TestEvidenceRunnerLocal: a run admitted only on this host ran here, as a process, with no attempt records.
func TestEvidenceRunnerLocal(t *testing.T) {
	o := admit(t, "rl", "build", 1, "local", true, nil)
	ev := runnerEvidence(t, "rl", []JobOutcome{o})
	want := model.RunnerRef{ID: "local", Provider: "local", Platform: runtime.GOOS + "/" + runtime.GOARCH, Isolation: "process"}
	if ev.Runner != want {
		t.Errorf("runner %+v, want %+v", ev.Runner, want)
	}
	if len(ev.Attempts) != 0 {
		t.Errorf("a local run has no attempt records, got %+v", ev.Attempts)
	}
}

// TestEvidenceRunnerRemote: a run placed on one node names that node, its platform and isolation, and lists the attempts.
func TestEvidenceRunnerRemote(t *testing.T) {
	d := remoteRunner("lab-arm")
	a := admit(t, "rr", "test", 1, "lab-arm", false, &d)
	b := admit(t, "rr", "build", 1, "lab-arm", false, &d)
	ev := runnerEvidence(t, "rr", []JobOutcome{a, b, {ID: "docs", State: StateSkipped}})
	want := model.RunnerRef{ID: "lab-arm", Provider: "ssh", Platform: "linux/arm64", Isolation: "container"}
	if ev.Runner != want {
		t.Errorf("runner %+v, want %+v", ev.Runner, want)
	}
	if got := nodes(ev); got != "build#1@lab-arm test#1@lab-arm" {
		t.Errorf("attempts %s", got)
	}
	if r := ev.Attempts[0].Runner; r.Platform != "linux/arm64" || r.Isolation != "container" || r.Kind != "ssh" {
		t.Errorf("attempt runner %+v", r)
	}
}

// TestEvidenceRunnerRetryMovesNode: attempt 1 ran on one runner and its infra retry on another. Only the
// last attempt is a job outcome, yet both are listed and no single runner is named for the run.
func TestEvidenceRunnerRetryMovesNode(t *testing.T) {
	a, b := remoteRunner("lab-arm"), remoteRunner("lab-x86")
	admit(t, "rt", "test", 1, "lab-arm", false, &a)
	final := admit(t, "rt", "test", 2, "lab-x86", false, &b)
	confirm := admit(t, "rt", "test", 3, "lab-arm", false, &a) // a confirmation rerun is no job outcome either
	_ = confirm
	ev := runnerEvidence(t, "rt", []JobOutcome{final})
	if got, want := nodes(ev), "test#1@lab-arm test#2@lab-x86 test#3@lab-arm"; got != want {
		t.Errorf("attempts %s, want %s", got, want)
	}
	if ev.Runner != CoordinatorRunner() {
		t.Errorf("two runners executed the job: the run-level runner is the coordinator, got %+v", ev.Runner)
	}
}

// TestEvidenceRunnerMixed: a placed job and a job that ran here have no single runner either.
func TestEvidenceRunnerMixed(t *testing.T) {
	d := remoteRunner("lab-arm")
	a := admit(t, "rm", "build", 1, "lab-arm", false, &d)
	b := admit(t, "rm", "lint", 1, "local", true, nil)
	ev := runnerEvidence(t, "rm", []JobOutcome{a, b})
	if ev.Runner != CoordinatorRunner() {
		t.Errorf("mixed run names the coordinator host, got %+v", ev.Runner)
	}
	if got := nodes(ev); got != "build#1@lab-arm lint#1@local" {
		t.Errorf("attempts %s", got)
	}
}

// TestEvidenceRunnerUnknownIsNeverLocal: a dispatched attempt whose runner details were never noted (the
// telemetry that carries them failed) is written as unknown, naming the node the lease was granted to.
func TestEvidenceRunnerUnknownIsNeverLocal(t *testing.T) {
	o := admit(t, "ru", "test", 1, "lab-arm", false, nil)
	ev := runnerEvidence(t, "ru", []JobOutcome{o})
	if ev.Runner.ID != "lab-arm" || ev.Runner.Platform != "unknown" || ev.Runner.Provider != "unknown" {
		t.Errorf("run-level runner %+v, want lab-arm with unknown platform and provider", ev.Runner)
	}
	if len(ev.Attempts) != 1 || ev.Attempts[0].Runner.Kind != "unknown" || ev.Attempts[0].Runner.Node != "lab-arm" {
		t.Errorf("attempts %+v", ev.Attempts)
	}
	if ev.Runner == CoordinatorRunner() || ev.Attempts[0].Runner.Kind == "local" {
		t.Error("a dispatched attempt must never be recorded as local")
	}
}
