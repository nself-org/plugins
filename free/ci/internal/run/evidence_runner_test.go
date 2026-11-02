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

func runnerEvidence(t *testing.T, jobs []JobOutcome) model.Evidence {
	t.Helper()
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	ev, _, err := buildEvidence(evidenceInput{Revision: strings.Repeat("a", 40), Start: start, End: start, Trigger: "local", Jobs: jobs,
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

func executed(id, attempt string) JobOutcome {
	return JobOutcome{ID: id, AttemptID: attempt, N: 1, State: StatePassed, Required: true}
}

// TestEvidenceRunnerLocal: a run with no placement note ran on this host, as a process.
func TestEvidenceRunnerLocal(t *testing.T) {
	ev := runnerEvidence(t, []JobOutcome{executed("build", "run.build.a1")})
	want := model.RunnerRef{ID: "local", Provider: "local", Platform: runtime.GOOS + "/" + runtime.GOARCH, Isolation: "process"}
	if ev.Runner != want {
		t.Errorf("runner %+v, want %+v", ev.Runner, want)
	}
	if len(ev.Attempts) != 0 {
		t.Errorf("a local run has no attempt records, got %+v", ev.Attempts)
	}
}

// TestEvidenceRunnerRemote: a run placed on one node names that node, its platform and isolation,
// and lists the attempt; a skipped job is not an attempt.
func TestEvidenceRunnerRemote(t *testing.T) {
	NoteRunner("run.build.a1", remoteRunner("lab-arm"))
	NoteRunner("run.test.a1", remoteRunner("lab-arm"))
	t.Cleanup(func() { ForgetRunner("run.build.a1"); ForgetRunner("run.test.a1") })
	ev := runnerEvidence(t, []JobOutcome{executed("test", "run.test.a1"), executed("build", "run.build.a1"), {ID: "docs", State: StateSkipped}})
	want := model.RunnerRef{ID: "lab-arm", Provider: "ssh", Platform: "linux/arm64", Isolation: "container"}
	if ev.Runner != want {
		t.Errorf("runner %+v, want %+v", ev.Runner, want)
	}
	if len(ev.Attempts) != 2 || ev.Attempts[0].JobID != "build" || ev.Attempts[1].JobID != "test" {
		t.Fatalf("attempts %+v, want build then test", ev.Attempts)
	}
	if a := ev.Attempts[0].Runner; a.Node != "lab-arm" || a.Platform != "linux/arm64" || a.Isolation != "container" || a.Kind != "ssh" {
		t.Errorf("attempt runner %+v", a)
	}
}

// TestEvidenceRunnerMixed: jobs on different runners (or one placed, one local) have no single
// runner; the coordinator host is the run-level runner and every placed attempt names its own.
func TestEvidenceRunnerMixed(t *testing.T) {
	NoteRunner("run.build.a1", remoteRunner("lab-arm"))
	NoteRunner("run.test.a1", remoteRunner("lab-x86"))
	t.Cleanup(func() { ForgetRunner("run.build.a1"); ForgetRunner("run.test.a1") })
	ev := runnerEvidence(t, []JobOutcome{executed("build", "run.build.a1"), executed("test", "run.test.a1"), executed("lint", "run.lint.a1")})
	if ev.Runner != CoordinatorRunner() {
		t.Errorf("a mixed run names the coordinator host, got %+v", ev.Runner)
	}
	if len(ev.Attempts) != 2 || ev.Attempts[0].Runner.Node != "lab-arm" || ev.Attempts[1].Runner.Node != "lab-x86" {
		t.Errorf("attempts %+v", ev.Attempts)
	}
	ForgetRunner("run.test.a1") // one remote placement plus an unplaced (local) job is also mixed
	if ev = runnerEvidence(t, []JobOutcome{executed("build", "run.build.a1"), executed("test", "run.test.a1")}); ev.Runner != CoordinatorRunner() {
		t.Errorf("a placed and an unplaced job: %+v", ev.Runner)
	}
}
