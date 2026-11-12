package security

// Purpose: scenario 7 at the run.Coordinator (P7-WIRE-03): a pull request is admitted by the
// coordinator itself, not by the dispatcher rig. A fork PR runs only on a disposable hosted
// runner (Option A of ADR 0025 §2 and §6) and then with no secrets, no commit status and a
// restricted network; the same PR on a persistent runner is refused. A same-repo admin PR (class
// collaborator) follows the same rule.
// Inputs:  a real run.Coordinator over a real plan and state store; the runner kind is the
//          process environment the coordinator reads (GITHUB_ACTIONS, RUNNER_ENVIRONMENT).
// Outputs: pass or fail.
// Constraints: no build tag, so the plain package run executes it; the trust class is returned by
//          a Hooks.Trust classifier because the forge lookup needs the network (the class mapping
//          itself is tested in cmd: TestWire03SamePRAdminClass).

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

const wire03SHA = "0123456789abcdef0123456789abcdef01234567"
const wire03Secret = "s3cr3t-value-wire03"

type wire03Class model.TrustClass

func (c wire03Class) Classify(context.Context, cirun.TrustRequest) (model.TrustClass, error) {
	return model.TrustClass(c), nil
}

type wire03Reporter struct {
	mu sync.Mutex
	n  int
}

func (r *wire03Reporter) Post(context.Context, report.Status) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.n++
	return nil
}

func wire03Capture(_ context.Context, argv ...string) (string, int, error) {
	j := strings.Join(argv, " ")
	switch {
	case strings.Contains(j, "rev-parse HEAD"):
		return wire03SHA + "\n", 0, nil
	case strings.Contains(j, "remote get-url"):
		return "git@github.com:acme/widgets.git\n", 0, nil
	case strings.Contains(j, "gh auth status"):
		return "ok\n", 0, nil
	}
	return "", 0, nil
}

// wire03Run runs a pull-request plan under class and returns the result, the error, the output and
// the number of commit statuses the reporter was asked to post.
func wire03Run(t *testing.T, class model.TrustClass, hosted bool) (cirun.Result, error, string, int) {
	t.Helper()
	env := map[string]string{"GITHUB_ACTIONS": "", "RUNNER_ENVIRONMENT": "", "DEPLOY_TOKEN": wire03Secret}
	if hosted {
		env["GITHUB_ACTIONS"], env["RUNNER_ENVIRONMENT"] = "true", "github-hosted"
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	job := model.Job{Kind: "test", Required: &[]bool{true}[0], Secrets: []string{"DEPLOY_TOKEN"},
		Run: []string{"sh", "-c", `echo "token=[$DEPLOY_TOKEN]"`}}
	p, err := plan.Build(plan.Input{Config: model.PipelineConfig{Version: 1, Jobs: map[string]model.Job{"check": job}}, Trigger: "pull_request",
		World: plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, CPU: 8, MemMB: 16384}})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	out, rep := &bytes.Buffer{}, &wire03Reporter{}
	c := &cirun.Coordinator{Opts: cirun.Options{Root: t.TempDir(), Home: t.TempDir(), Network: "lan", Trigger: "pull_request"}, Plan: p, ConfigDigest: "cfg",
		Hooks: cirun.Hooks{Trust: wire03Class(class)}, Out: out, Capture: wire03Capture, Reporter: rep, Poll: 10 * time.Millisecond,
		Capacity: func() ciexec.Capacity {
			return ciexec.Capacity{CPUs: 8, MemoryMB: 16384, FreeMemoryMB: 12000, BatteryPercent: -1, PluggedIn: true, Interactive: true}
		}}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	res, err := c.Run(ctx)
	rep.mu.Lock()
	defer rep.mu.Unlock()
	return res, err, out.String(), rep.n
}

func wire03AssertContained(t *testing.T, res cirun.Result, out string, posts int) {
	t.Helper()
	if posts != 0 {
		t.Errorf("%d commit status posts for a pull request run, want 0", posts)
	}
	if strings.Contains(out, wire03Secret) {
		t.Errorf("the repo secret reached the job output:\n%s", out)
	}
	if res.RunID == "" {
		t.Errorf("the run was not admitted: %+v", res)
	}
}

func TestWire03ForkPRHostedUntrusted(t *testing.T) {
	res, err, out, posts := wire03Run(t, "untrusted", true)
	if err != nil {
		t.Fatalf("a fork PR on a disposable hosted runner was refused: %v\n%s", err, out)
	}
	wire03AssertContained(t, res, out, posts)
}

func TestWire03SamePRAdminRunsWithoutSecrets(t *testing.T) {
	res, err, out, posts := wire03Run(t, "collaborator", true)
	if err != nil {
		t.Fatalf("a same-repo admin PR on a disposable hosted runner was refused: %v\n%s", err, out)
	}
	wire03AssertContained(t, res, out, posts)
}

func TestWire03ForkPROnPersistentRunnerRefused(t *testing.T) {
	for _, class := range []model.TrustClass{"untrusted", "collaborator"} {
		res, err, out, posts := wire03Run(t, class, false)
		if err == nil || !strings.Contains(err.Error(), "isolated runner") {
			t.Fatalf("%s PR on a persistent runner: err = %v, want a trust refusal\n%s", class, err, out)
		}
		if res.RunID != "" || posts != 0 || strings.Contains(out, wire03Secret) {
			t.Errorf("%s PR on a persistent runner started work: run %q, %d posts", class, res.RunID, posts)
		}
	}
}
