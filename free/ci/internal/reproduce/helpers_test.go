package reproduce

// Shared fixtures: a real git repository, a real run of a shell job through
// the Coordinator (so the evidence is the engine's own, signed), and the
// RunFunc cmd supplies, built on the same Coordinator.

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// TestMain points NSELF_CI_HOME at a throwaway directory: the executor's
// orphan sweep reads it and no test may touch the user's ~/.nself/ci.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "ci41-home-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("NSELF_CI_HOME", home)
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@example.test", "-c", "user.name=t", "-c", "commit.gpgsign=false", "-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newRepo makes a repository with one commit.
func newRepo(t *testing.T) (dir, sha string) {
	t.Helper()
	dir = t.TempDir()
	gitRun(t, dir, "init", "-q", "-b", "main")
	writeFile(t, dir, "go.mod", "module x\n\ngo 1.22\n")
	writeFile(t, dir, "go.sum", "h1:one\n")
	writeFile(t, dir, "a.txt", "committed\n")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", "init")
	return dir, gitRun(t, dir, "rev-parse", "HEAD")
}

func yes() *bool { b := true; return &b }

// failingJobs is one required shell job that fails with exit 3.
func failingJobs() map[string]model.Job {
	return map[string]model.Job{"unit": {Kind: "test", Run: []string{"sh", "-c", "echo boom; exit 3"}, Required: yes()}}
}

func buildPlan(t *testing.T, jobs map[string]model.Job) *plan.Plan {
	t.Helper()
	p, err := plan.Build(plan.Input{Config: model.PipelineConfig{Version: 1, Jobs: jobs},
		World: plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, CPU: 8, MemMB: 16384}})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return p
}

func roomy() ciexec.Capacity {
	return ciexec.Capacity{CPUs: 8, MemoryMB: 16384, FreeMemoryMB: 12000, BatteryPercent: -1, PluggedIn: true, Interactive: true}
}

func runIn(t *testing.T, root, home, job string, p *plan.Plan) cirun.Result {
	t.Helper()
	c := &cirun.Coordinator{Opts: cirun.Options{Root: root, Home: home, NoStatus: true, Job: job}, Plan: p, ConfigDigest: "cfg",
		Out: io.Discard, Capacity: roomy, Poll: 10 * time.Millisecond}
	res, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res
}

// engine is the RunFunc of cmd, on the Coordinator; seen records the specs.
type engine struct {
	t     *testing.T
	home  string
	p     *plan.Plan
	mu    sync.Mutex
	seen  []RunSpec
	trees []string
}

func (e *engine) run(ctx context.Context, spec RunSpec) (RunOutcome, error) {
	e.mu.Lock()
	e.seen = append(e.seen, spec)
	e.mu.Unlock()
	c := &cirun.Coordinator{Opts: cirun.Options{Root: spec.Tree, Home: e.home, NoStatus: true, Job: spec.Job}, Plan: e.p, ConfigDigest: "cfg",
		Out: io.Discard, Capacity: roomy, Poll: 10 * time.Millisecond}
	res, err := c.Run(ctx)
	if err != nil {
		return RunOutcome{}, err
	}
	return RunOutcome{RunID: res.RunID, Exit: res.Exit, Evidence: res.Summary.Doc}, nil
}

func openStore(t *testing.T, home string) *store.Store {
	t.Helper()
	st, err := cirun.OpenStore(filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// worktrees lists the worktree paths git knows for repo.
func worktrees(t *testing.T, repo string) []string {
	t.Helper()
	var out []string
	for _, l := range strings.Split(gitRun(t, repo, "worktree", "list", "--porcelain"), "\n") {
		if p, ok := strings.CutPrefix(l, "worktree "); ok {
			out = append(out, p)
		}
	}
	return out
}
