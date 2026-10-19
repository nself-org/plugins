package main

// P7-CI-39 through the real command path: a run stores its output redacted,
// `ci logs` prints it (text and NDJSON), `ci runs list` and `ci runs prune`
// read and trim the history.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/logs"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

const logsSecret = "cmd-log-secret-a1b2c3d4e5"

// logsRepo is a repository whose one job prints a planted secret between two lines.
func logsRepo(t *testing.T) *evRepo {
	t.Helper()
	if err := cirun.RegisterSecretValue([]byte(logsSecret)); err != nil {
		t.Fatal(err)
	}
	r := newEvRepo(t)
	r.commit(".nself/ci.yaml", "version: 1\njobs:\n  unit:\n    kind: test\n    run: [\"sh\", \"-c\", \"echo alpha; echo leak:"+logsSecret+"; echo omega\"]\n")
	if _, errOut, code := r.ci("--no-status"); code != 0 {
		t.Fatalf("run failed (%d): %s", code, errOut)
	}
	return r
}

func TestLogsPrintsRedactedOutputOfAJob(t *testing.T) {
	r := logsRepo(t)
	out, errOut, code := r.ci("logs", "unit")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if strings.Contains(out, logsSecret) || !strings.Contains(strings.ToLower(out), "[redacted]") {
		t.Fatalf("planted secret in the output (or no mask):\n%s", out)
	}
	if lines := strings.Split(strings.TrimSpace(out), "\n"); len(lines) != 3 || lines[0] != "alpha" || lines[2] != "omega" {
		t.Fatalf("lines: %q", lines)
	}
}

func TestLogsJSONIsNDJSONWithAResultLast(t *testing.T) {
	r := logsRepo(t)
	out, errOut, code := r.ci("logs", "unit", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if strings.Contains(out, logsSecret) {
		t.Fatalf("planted secret in --json output:\n%s", out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("want 3 log records and a result, got %d lines:\n%s", len(lines), out)
	}
	for i, l := range lines {
		if err := output.CheckEnvelope([]byte(l)); err != nil {
			t.Fatalf("line %d is not a v1 envelope: %v", i, err)
		}
		var env struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal([]byte(l), &env)
		wantType := "log"
		if i == 3 {
			wantType = "result"
		}
		if env.Data["type"] != wantType {
			t.Fatalf("line %d type %v, want %s", i, env.Data["type"], wantType)
		}
		if i < 3 && int(env.Data["seq"].(float64)) != i+1 {
			t.Fatalf("line %d seq %v", i, env.Data["seq"])
		}
	}
}

func TestLogsFollowOnAFinishedAttemptReturns(t *testing.T) {
	r := logsRepo(t)
	out, _, code := r.ci("logs", "unit", "-f")
	if code != 0 || !strings.Contains(out, "omega") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
}

func TestLogsResolvesAnAttemptIdAndRefusesStrangers(t *testing.T) {
	r := logsRepo(t)
	runs, _ := logs.ListRuns(r.home)
	att, _ := logs.Attempts(logs.RunDir(r.home, runs[0].Run))
	if out, _, code := r.ci("logs", att[0][:12]); code != 0 || !strings.Contains(out, "alpha") {
		t.Fatalf("by attempt prefix: exit %d\n%s", code, out)
	}
	for _, args := range [][]string{{"logs", "nope"}, {"logs", "../../etc/passwd"}, {"logs", "unit", "--attempt", "9"}, {"logs"}} {
		_, errOut, code := r.ci(args...)
		if code != 1 {
			t.Errorf("%v: exit %d, want 1 (%s)", args, code, errOut)
		}
	}
	if _, errOut, _ := r.ci("logs", "nope"); !strings.Contains(errOut, logs.CodeNone) {
		t.Errorf("want E633 in %q", errOut)
	}
	if out, _, code := r.ci("logs", "nope", "--json"); code != 1 || !strings.Contains(out, `"E633"`) {
		t.Errorf("--json error: exit %d %s", code, out)
	}
}

func TestLogsAsksRemoteResolversFirst(t *testing.T) {
	r := logsRepo(t)
	saved := cirun.Registered.Remote
	cirun.RegisterRemote(remoteRun{})
	defer func() { cirun.Registered.Remote = saved }()
	_, errOut, code := r.ci("logs", "unit", "--run", "remote-run")
	if code != 1 || !strings.Contains(errOut, "remote-run") || !strings.Contains(errOut, logs.CodeNone) {
		t.Fatalf("exit %d: %s", code, errOut)
	}
}

func TestRunsListShowsHistoryTextAndJSON(t *testing.T) {
	r := logsRepo(t)
	out, _, code := r.ci("runs", "list")
	if code != 0 || !strings.Contains(out, "RESULT") || !strings.Contains(out, r.head()[:7]) || !strings.Contains(out, "pass") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	rows, errOut, code := r.ci("runs", "list", "--json", "--limit", "5")
	if err := output.CheckEnvelope([]byte(rows)); err != nil {
		t.Fatalf("not a v1 envelope: %v\n%s\n%s", err, rows, errOut)
	}
	var env struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(rows), &env); err != nil || code != 0 || len(env.Data) != 1 {
		t.Fatalf("exit %d, %v: %s", code, err, rows)
	}
	e := env.Data[0]
	if e["result"] != "pass" || e["binding"] != "bound" || e["freshness"] != "current" || e["revision"] != r.head() || e["duration_ms"] == nil || e["log_bytes"].(float64) == 0 {
		t.Fatalf("entry: %v", e)
	}
	if _, _, code = r.ci("runs", "list", "--limit", "0"); code != 1 {
		t.Errorf("--limit 0: exit %d, want 1", code)
	}
}

func TestRunsPruneTrimsHistoryAndLogsTogether(t *testing.T) {
	r := logsRepo(t)
	r.commit("x.txt", "second")
	if _, errOut, code := r.ci("--no-status"); code != 0 {
		t.Fatalf("second run: %d %s", code, errOut)
	}
	if err := os.WriteFile(filepath.Join(r.home, "retention.json"), []byte(`{"keep_runs": 1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, _, code := r.ci("runs", "prune", "--dry-run"); code != 0 || !strings.Contains(out, "would remove 1 run") || r.runCount() != 2 {
		t.Fatalf("dry run: exit %d, %d runs\n%s", code, r.runCount(), out)
	}
	out, _, code := r.ci("runs", "prune")
	if code != 0 || !strings.Contains(out, "removed 1 run") || r.runCount() != 1 {
		t.Fatalf("prune: exit %d, %d runs\n%s", code, r.runCount(), out)
	}
	if dirs, _ := logs.ListRuns(r.home); len(dirs) != 1 {
		t.Fatalf("log dirs after prune: %+v", dirs)
	}
	if _, _, code = r.ci("runs", "prune", "--keep", "-1"); code != 1 {
		t.Errorf("--keep -1: exit %d, want 1", code)
	}
}
