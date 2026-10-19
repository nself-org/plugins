package drill

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

	"github.com/nself-org/plugins/free/ci/internal/config"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// TestMain keeps every run's state out of the user's real ~/.nself/ci.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "ci02-home-")
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

// fixtureRepo copies testdata/repo into a fresh git repository with one commit.
func fixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("cp", "-R", filepath.Join("testdata", "repo")+string(filepath.Separator)+".", dir).CombinedOutput(); err != nil {
		t.Fatalf("copy fixture: %v\n%s", err, out)
	}
	gitRun(t, dir, "init", "-q", "-b", "main")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", "init")
	return dir
}

func addDrill(t *testing.T, dir, name, script string) {
	t.Helper()
	d := filepath.Join(dir, "scripts", "drill", name)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, RunFile), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// planDrills loads the fixture config, expands the drills pipeline and plans it.
func planDrills(t *testing.T, dir string) (*plan.Plan, model.PipelineConfig) {
	t.Helper()
	loaded, err := config.Load(filepath.Join(dir, config.FileName))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cfg, err := Expand(dir, loaded.Config, "drills", "")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	p, err := plan.Build(plan.Input{Config: cfg, Pipeline: "drills",
		World: plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, CPU: 8, MemMB: 16384}})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return p, cfg
}

func roomy() ciexec.Capacity {
	return ciexec.Capacity{CPUs: 8, MemoryMB: 16384, FreeMemoryMB: 12000, BatteryPercent: -1, PluggedIn: true, Interactive: true}
}

// runDrills runs p in dir with the real exec funnel and the registered hooks.
func runDrills(t *testing.T, dir string, p *plan.Plan) cirun.Result {
	t.Helper()
	home := os.Getenv("NSELF_CI_HOME")
	c := &cirun.Coordinator{Opts: cirun.Options{Root: dir, Home: home, NoStatus: true}, Plan: p, ConfigDigest: "cfg",
		Out: io.Discard, Capacity: roomy, Poll: 10 * time.Millisecond}
	res, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res
}

func checksByID(ev model.Evidence) map[string]model.Check {
	out := map[string]model.Check{}
	for _, c := range ev.Checks {
		out[c.ID] = c
	}
	return out
}
