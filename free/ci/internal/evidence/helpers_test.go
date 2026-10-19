package evidence

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// git runs git in dir with a fixed identity and fails the test on error.
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

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newRepo makes a repository with one commit holding files.
func newRepo(t *testing.T, files map[string]string) (dir, sha string) {
	t.Helper()
	dir = t.TempDir()
	gitRun(t, dir, "init", "-q", "-b", "main")
	for n, b := range files {
		write(t, dir, n, b)
	}
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", "init")
	return dir, gitRun(t, dir, "rev-parse", "HEAD")
}

// commitFiles adds one commit that rewrites files.
func commitFiles(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	for n, b := range files {
		write(t, dir, n, b)
	}
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", "change")
	return gitRun(t, dir, "rev-parse", "HEAD")
}

var baseFiles = map[string]string{
	"go.mod": "module x\n\ngo 1.22\n",
	"go.sum": "h1:one\n",
	"a.txt":  "a\n",
}

func yes() *bool { y := true; return &y }

func jobsOf(run ...string) map[string]model.Job {
	return map[string]model.Job{"unit": {Kind: "test", Run: run, Required: yes()}}
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

// runPipeline runs p in dir with the real exec funnel and the registered
// evidence builder, and returns the result with the run store.
func runPipeline(t *testing.T, dir, home string, p *plan.Plan, hooks cirun.Hooks) (cirun.Result, *store.Store) {
	t.Helper()
	c := &cirun.Coordinator{Opts: cirun.Options{Root: dir, Home: home, NoStatus: true}, Plan: p, ConfigDigest: "cfg", Hooks: hooks,
		Out: io.Discard, Capacity: roomy, Poll: 10 * time.Millisecond}
	res, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	st, err := cirun.OpenStore(filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return res, st
}

func ok(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := marshal(v)
	ok(t, err)
	return b
}

func equalInputs(a, b model.InputDigests) bool {
	return string(mustJSONRaw(a)) == string(mustJSONRaw(b))
}

func mustJSONRaw(v any) []byte { b, _ := marshal(v); return b }

// evOf is a minimal document bound to d.
func evOf(d model.InputDigests) model.Evidence { return model.Evidence{InputDigests: d} }
