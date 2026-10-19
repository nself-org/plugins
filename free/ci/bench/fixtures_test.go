package bench

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/config"
)

func TestMaterialize(t *testing.T) {
	for _, name := range []string{Tiny, Medium} {
		dir := filepath.Join(t.TempDir(), name)
		if err := Materialize(context.Background(), name, dir); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, f := range []string{"go.mod", config.FileName, ".git/HEAD"} {
			if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
				t.Errorf("%s: %v", name, err)
			}
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod.fixture")); err == nil {
			t.Errorf("%s: the .fixture suffix was kept", name)
		}
		loaded, err := config.Load(filepath.Join(dir, config.FileName))
		if err != nil {
			t.Fatalf("%s: V15 config does not load: %v", name, err)
		}
		if got := loaded.Config.Pipelines["gate"].Jobs; len(got) < 3 {
			t.Errorf("%s: gate pipeline lists %v", name, got)
		}
		r, err := must(context.Background(), []string{"git", "log", "--oneline"}, dir, gitEnv())
		if err != nil || r.Tail == "" {
			t.Errorf("%s: no commit: %v", name, err)
		}
	}
}

func TestInprocScenariosRun(t *testing.T) {
	dir := t.TempDir()
	if err := Materialize(context.Background(), Tiny, dir+"/repo"); err != nil {
		t.Fatal(err)
	}
	p, err := PrepareInproc(dir+"/repo", dir+"/scratch")
	if err != nil {
		t.Fatal(err)
	}
	for name, fn := range map[string]func() error{"detect": p.Detect, "plan": p.Plan, "store": p.StoreOpen} {
		if err := fn(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if len(p.facts) == 0 {
		t.Error("detect found no facts in a Go module")
	}
	if _, err := p.ExecOverhead(context.Background()); err != nil {
		t.Errorf("exec overhead: %v", err)
	}
	if r, err := p.RawSpawn(context.Background()); err != nil || r.Wall <= 0 {
		t.Errorf("raw spawn: %+v %v", r, err)
	}
}

func TestSpawnReportsExitCode(t *testing.T) {
	r, err := spawn(context.Background(), []string{"sh", "-c", "echo boom; exit 3"}, "", baseEnv())
	if err != nil || r.ExitCode != 3 || r.Tail != "boom" || r.Wall <= 0 {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err := must(context.Background(), []string{"sh", "-c", "exit 1"}, "", baseEnv()); err == nil {
		t.Fatal("must accepted exit 1")
	}
	if _, err := spawn(context.Background(), []string{"/nonexistent/program"}, "", nil); err == nil {
		t.Fatal("a program that cannot start must be an error")
	}
	if _, err := spawn(context.Background(), nil, "", nil); err == nil {
		t.Fatal("empty argv accepted")
	}
}
