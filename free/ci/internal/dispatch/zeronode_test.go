package dispatch_test

// TestZeroNode: with the dispatcher registered (NSELF_V15=1) and no nodes or
// providers, every job of a real coordinator run is admitted to the local
// runner through dispatch.Admit and ends exactly as the same run does without
// the dispatcher (P7-CI-37's local run).

import (
	"bytes"
	"context"
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
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const zeroSHA = "0123456789abcdef0123456789abcdef01234567"

func zeroGit(_ context.Context, argv ...string) (string, int, error) {
	j := strings.Join(argv, " ")
	switch {
	case strings.Contains(j, "rev-parse HEAD"):
		return zeroSHA + "\n", 0, nil
	case strings.Contains(j, "remote get-url"):
		return "git@github.com:acme/widgets.git\n", 0, nil
	case strings.Contains(j, "gh auth status"):
		return "ok\n", 0, nil
	}
	return "", 0, nil
}

type zeroReporter struct{}

func (*zeroReporter) Post(context.Context, report.Status) error { return nil }

func shJob(script string, deps ...string) model.Job {
	yes := true
	return model.Job{Kind: "test", Run: []string{"sh", "-c", script}, Deps: deps, Required: &yes}
}

type zeroRun struct {
	res  run.Result
	home string
}

func zeroNodeRun(t *testing.T, v15 bool) zeroRun {
	t.Helper()
	if v15 {
		t.Setenv("NSELF_V15", "1")
	} else {
		t.Setenv("NSELF_V15", "")
	}
	jobs := map[string]model.Job{"build": shJob("echo build"), "test": shJob("echo test", "build"), "lint": shJob("echo lint"), "bad": shJob("exit 3")}
	jobs["bad"] = func() model.Job { j := jobs["bad"]; no := false; j.Required = &no; j.AllowFailure = true; return j }()
	pl, err := plan.Build(plan.Input{Config: model.PipelineConfig{Version: 1, Jobs: jobs}, World: plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, CPU: 8, MemMB: 16384}})
	if err != nil {
		t.Fatal(err)
	}
	root, home := t.TempDir(), t.TempDir()
	if err = os.WriteFile(filepath.Join(root, "f.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	c := &run.Coordinator{Opts: run.Options{Root: root, Home: home}, Plan: pl, ConfigDigest: "cfg", Out: out, Capture: zeroGit, Reporter: &zeroReporter{},
		Capacity: func() ciexec.Capacity {
			return ciexec.Capacity{CPUs: 8, MemoryMB: 16384, FreeMemoryMB: 12000, BatteryPercent: -1, PluggedIn: true, Interactive: true}
		}, Poll: 10 * time.Millisecond}
	res, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	return zeroRun{res, home}
}

func TestZeroNode(t *testing.T) {
	plain := zeroNodeRun(t, false)
	engaged := zeroNodeRun(t, true)
	if plain.res.Exit != engaged.res.Exit || plain.res.Summary.Result != engaged.res.Summary.Result {
		t.Fatalf("verdicts differ: plain %d/%s, dispatcher %d/%s", plain.res.Exit, plain.res.Summary.Result, engaged.res.Exit, engaged.res.Summary.Result)
	}
	if len(engaged.res.Summary.Jobs) != 4 || len(plain.res.Summary.Jobs) != 4 {
		t.Fatalf("jobs: %d and %d", len(plain.res.Summary.Jobs), len(engaged.res.Summary.Jobs))
	}
	byID := map[string]run.JobOutcome{}
	for _, j := range plain.res.Summary.Jobs {
		byID[j.ID] = j
	}
	st, err := store.Open(filepath.Join(engaged.home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	for _, j := range engaged.res.Summary.Jobs {
		p := byID[j.ID]
		if j.State != p.State || j.Reason != p.Reason || j.FailureClass != p.FailureClass || j.ExitCode != p.ExitCode || j.N != p.N {
			t.Errorf("job %s differs: dispatcher %+v, plain %+v", j.ID, j, p)
		}
		_, _, _, runner, digest, ok, err := st.AdmittedIdentity(context.Background(), j.AttemptID)
		if err != nil || !ok || runner != "local" || digest == "" {
			t.Errorf("job %s was not admitted through dispatch.Admit on the local runner: %q %q %v %v", j.ID, runner, digest, ok, err)
		}
	}
	// the control: without the dispatcher nothing records an admission identity
	pst, err := store.Open(filepath.Join(plain.home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pst.Close() }()
	for _, j := range plain.res.Summary.Jobs {
		if _, _, _, _, _, ok, _ := pst.AdmittedIdentity(context.Background(), j.AttemptID); ok {
			t.Errorf("control run recorded an admission for %s", j.ID)
		}
	}
}
