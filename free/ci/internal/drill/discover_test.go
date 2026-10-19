package drill

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/config"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestDiscoverSortsAndSkipsNonDrills(t *testing.T) {
	dir := fixtureRepo(t)
	addDrill(t, dir, "alpha", "#!/bin/sh\n")
	// A run file that is not executable, and a symlinked directory, are not drills.
	if runtime.GOOS != "windows" {
		d := filepath.Join(dir, "scripts", "drill", "plain")
		_ = os.MkdirAll(d, 0o755)
		_ = os.WriteFile(filepath.Join(d, RunFile), []byte("#!/bin/sh\n"), 0o644)
		_ = os.Symlink(filepath.Join(dir, "scripts", "drill", "alpha"), filepath.Join(dir, "scripts", "drill", "linked"))
	}
	got, err := Discover(dir, "scripts/drill")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"alpha", "fail-nested", "invalid", "pass"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("drills = %v, want %v", got, want)
	}
}

func TestDiscoverRefusals(t *testing.T) {
	dir := fixtureRepo(t)
	outside := t.TempDir()
	_ = os.Symlink(outside, filepath.Join(dir, "scripts", "escape"))
	badName := filepath.Join(dir, "scripts", "weird", "bad name")
	_ = os.MkdirAll(badName, 0o755)
	for _, tc := range []struct{ name, dir string }{
		{"missing", "scripts/nope"},
		{"escapes the repository", "scripts/escape"},
		{"bad directory name", "scripts/weird"},
	} {
		_, err := Discover(dir, tc.dir)
		var de *Error
		if !errors.As(err, &de) || de.Code != CodeInvalid {
			t.Errorf("%s: err = %v, want E640", tc.name, err)
		}
	}
}

func TestExpandAddsJobsInOrderWithoutMutating(t *testing.T) {
	dir := fixtureRepo(t)
	loaded, err := config.Load(filepath.Join(dir, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	before := len(loaded.Config.Jobs)
	cfg, err := Expand(dir, loaded.Config, "drills", "")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"drill-fail-nested", "drill-invalid", "drill-pass"}; !reflect.DeepEqual(cfg.Pipelines["drills"].Jobs, want) {
		t.Fatalf("pipeline jobs = %v, want %v", cfg.Pipelines["drills"].Jobs, want)
	}
	j := cfg.Jobs["drill-pass"]
	if j.Kind != "drill" || !reflect.DeepEqual(j.Run, []string{"./scripts/drill/pass/run"}) {
		t.Fatalf("job = %+v", j)
	}
	if len(loaded.Config.Jobs) != before || len(loaded.Config.Pipelines["drills"].Jobs) != 0 {
		t.Fatal("Expand mutated the loaded config")
	}
}

func TestExpandSelection(t *testing.T) {
	dir := fixtureRepo(t)
	loaded, _ := config.Load(filepath.Join(dir, config.FileName))
	cfg := loaded.Config
	for _, tc := range []struct {
		name, pipeline, trigger string
		want                    int
	}{
		{"by pipeline", "drills", "", 3},
		{"by trigger", "", "schedule", 3},
		{"other pipeline", "default", "", 0},
		{"no selection", "", "", 0},
		{"other trigger", "", "push", 0},
	} {
		out, err := Expand(dir, cfg, tc.pipeline, tc.trigger)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := len(out.Pipelines["drills"].Jobs); got != tc.want {
			t.Errorf("%s: %d jobs, want %d", tc.name, got, tc.want)
		}
	}
	// A pipeline that is not selected never fails on a missing drills_dir.
	cfg.Pipelines = map[string]model.PipelineDef{"drills": {Triggers: []model.Trigger{"manual"}, DrillsDir: "scripts/gone"}}
	if _, err := Expand(dir, cfg, "", ""); err != nil {
		t.Fatalf("unselected pipeline: %v", err)
	}
	if _, err := Expand(dir, cfg, "drills", ""); err == nil {
		t.Fatal("a selected pipeline with a missing drills_dir must fail")
	}
}

func TestExpandKeepsExplicitJobAndRefusesEmpty(t *testing.T) {
	dir := fixtureRepo(t)
	loaded, _ := config.Load(filepath.Join(dir, config.FileName))
	cfg := loaded.Config
	cfg.Jobs = map[string]model.Job{"drill-pass": {Kind: "drill", Run: []string{"./scripts/drill/pass/run", "--long"}, Timeout: "90m"}}
	out, err := Expand(dir, cfg, "drills", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Jobs["drill-pass"]; got.Timeout != "90m" || len(got.Run) != 2 {
		t.Fatalf("explicit job was replaced: %+v", got)
	}
	empty := t.TempDir()
	_ = os.MkdirAll(filepath.Join(empty, "scripts", "drill"), 0o755)
	if _, err = Expand(empty, cfg, "drills", ""); err == nil {
		t.Fatal("an empty drills_dir must fail, not pass silently")
	}
}
