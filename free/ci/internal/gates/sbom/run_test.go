package sbom_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	gsbom "github.com/nself-org/plugins/free/ci/internal/gates/sbom"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sbom"
)

const pinV = "1.54.1"

// scripted answers `<syft> version` and scans from fixed documents.
type scripted struct {
	version string // "" = syft does not run
	scan    func(c sbom.Cmd) (sbom.Result, error)
}

func (s scripted) IsolationName() string { return "process" }

// withIso reports another isolation than the wrapped runner's default.
type withIso struct {
	sbom.Runner
	iso string
}

func (w withIso) IsolationName() string { return w.iso }

func (s scripted) Run(_ context.Context, c sbom.Cmd) (sbom.Result, error) {
	if len(c.Argv) == 2 && c.Argv[1] == "version" {
		if s.version == "" {
			return sbom.Result{}, errors.New("no such file")
		}
		return sbom.Result{Output: "Version:       " + s.version + "\n"}, nil
	}
	return s.scan(c)
}

func goodScan(t *testing.T) func(sbom.Cmd) (sbom.Result, error) {
	cdx, err := os.ReadFile("../../sbom/testdata/golden/dir.cdx.json")
	if err != nil {
		t.Fatal(err)
	}
	spdx, err := os.ReadFile("../../sbom/testdata/golden/dir.spdx.json")
	if err != nil {
		t.Fatal(err)
	}
	return func(c sbom.Cmd) (sbom.Result, error) {
		name := c.Argv[3+1] // scan <src> --source-name <name>
		doc := strings.Replace(string(cdx), `"name": "fx"`, `"name": "`+name+`"`, 1)
		sp := strings.ReplaceAll(string(spdx), `"name": "fx"`, `"name": "`+name+`"`)
		return sbom.Result{Files: map[string][]byte{c.Collect[0]: []byte(doc), c.Collect[1]: []byte(sp)}}, nil
	}
}

func input(t *testing.T, kind model.JobKind, iso model.Isolation, r sbom.Runner) gsbom.Input {
	if _, ok := r.(withIso); !ok && iso != "" && iso != "process" {
		r = withIso{r, string(iso)} // the runner executes where the job does
	}
	home := t.TempDir()
	if iso != "container" {
		p := sbom.ToolPath(home, pinV)
		if err := os.MkdirAll(strings.TrimSuffix(p, "/syft"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return gsbom.Input{
		Job:     model.Job{Kind: kind, Isolation: iso, Run: []string{"make", gsbom.CheckID}},
		Runner:  r,
		Home:    home,
		Pin:     sbom.Pin{Version: pinV},
		Targets: []sbom.Target{{Name: "cli", Path: "dist/cli", Kind: sbom.SourceFile}},
	}
}

func TestRunPasses(t *testing.T) {
	in := input(t, "release", "container", scripted{version: pinV, scan: goodScan(t)})
	out := gsbom.Run(context.Background(), in)
	c := out.Check
	if c.Result != "pass" || !c.Required || !c.Substantive || c.Kind != "sbom" || c.ToolVersion == nil || *c.ToolVersion != pinV || len(out.Docs) != 1 {
		t.Fatalf("outcome = %+v docs=%d", c, len(out.Docs))
	}
}

func TestRunFailureOfAReleaseJobIsRequiredAndNamesE718(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*gsbom.Input)
		runner sbom.Runner
		reason string
	}{
		{"syft missing", nil, scripted{version: ""}, "tool_missing"},
		{"version differs", nil, scripted{version: "1.0.0"}, "tool_version"},
		{"invalid output", nil, scripted{version: pinV, scan: func(c sbom.Cmd) (sbom.Result, error) {
			return sbom.Result{Files: map[string][]byte{c.Collect[0]: []byte(`{}`), c.Collect[1]: []byte(`{}`)}}, nil
		}}, "invalid"},
		{"check not run", func(in *gsbom.Input) { in.Job.Run = []string{"make"} }, scripted{version: pinV, scan: goodScan(t)}, "invalid"},
	}
	for _, c := range cases {
		in := input(t, "release", "container", c.runner)
		if c.mutate != nil {
			c.mutate(&in)
		}
		out := gsbom.Run(context.Background(), in)
		got := out.Check
		if got.Result != "error" || !got.Required || got.FailureClass != "infra" || !strings.HasPrefix(got.Excerpt, "E718 "+c.reason+":") || len(out.Docs) != 0 {
			t.Errorf("%s: %+v docs=%d", c.name, got, len(out.Docs))
		}
	}
}

func TestRunFailureInABuildJobIsAdvisory(t *testing.T) {
	missing := gsbom.Run(context.Background(), input(t, "build", "container", scripted{version: ""})).Check
	if missing.Result != "skip" || missing.Required || missing.Reason != "advisory.tool_missing" || !strings.HasPrefix(missing.Excerpt, "E718 tool_missing:") {
		t.Fatalf("missing tool: %+v", missing)
	}
	drift := gsbom.Run(context.Background(), input(t, "build", "container", scripted{version: "1.0.0"})).Check
	if drift.Result != "skip" || drift.Required || !strings.HasPrefix(drift.Excerpt, "E718 tool_version:") {
		t.Fatalf("version drift: %+v", drift)
	}
	bad := scripted{version: pinV, scan: func(c sbom.Cmd) (sbom.Result, error) { return sbom.Result{Exit: 2, Output: "no good"}, nil }}
	invalid := gsbom.Run(context.Background(), input(t, "build", "container", bad)).Check
	if invalid.Result != "fail" || invalid.Required || !strings.HasPrefix(invalid.Excerpt, "E718 invalid:") {
		t.Fatalf("invalid output: %+v", invalid)
	}
	strict := input(t, "build", "container", scripted{version: ""})
	strict.Config = model.PipelineConfig{Presets: []model.Preset{"strict"}}
	if c := gsbom.Run(context.Background(), strict).Check; c.Result != "error" || !c.Required {
		t.Fatalf("strict preset: %+v", c)
	}
}

func TestRunProcessJobUsesTheInstalledSyft(t *testing.T) {
	var ran string
	r := scripted{version: pinV, scan: func(c sbom.Cmd) (sbom.Result, error) { ran = c.Argv[0]; return goodScan(t)(c) }}
	in := input(t, "release", "process", r)
	if c := gsbom.Run(context.Background(), in).Check; c.Result != "pass" || ran != sbom.ToolPath(in.Home, pinV) {
		t.Fatalf("result %+v, syft run = %q", c, ran)
	}
	// the home has no install: a process-level release job fails and names the command
	in.Home = t.TempDir()
	t.Setenv("PATH", t.TempDir())
	c := gsbom.Run(context.Background(), in).Check
	if c.Result != "error" || !strings.Contains(c.Excerpt, sbom.InstallCommand) {
		t.Fatalf("uninstalled: %+v", c)
	}
}

func TestRunRefusesAJobAndRunnerThatDisagreeOnIsolation(t *testing.T) {
	for _, c := range []struct {
		job    model.Isolation
		runner string
	}{{"container", "process"}, {"process", "container"}, {"", "container"}, {"sandboxed-container", "container"}, {"vm", "container"}} {
		in := input(t, "release", c.job, withIso{scripted{version: pinV, scan: goodScan(t)}, c.runner})
		got := gsbom.Run(context.Background(), in).Check
		if got.Result != "error" || !strings.Contains(got.Excerpt, "isolation") {
			t.Errorf("job %q runner %q: %+v", c.job, c.runner, got)
		}
	}
	in := input(t, "release", "sandboxed-container", withIso{scripted{version: pinV, scan: goodScan(t)}, "sandboxed-container"})
	if got := gsbom.Run(context.Background(), in).Check; got.Result != "pass" {
		t.Fatalf("matching sandboxed-container: %+v", got)
	}
}

func TestRunWithoutARunnerFailsClosed(t *testing.T) {
	in := input(t, "release", "process", scripted{})
	in.Runner = nil
	if got := gsbom.Run(context.Background(), in).Check; got.Result != "error" || !strings.HasPrefix(got.Excerpt, "E718 tool_missing:") {
		t.Fatalf("nil runner: %+v", got)
	}
}

// TestRunNeverRunsAToolPlantedInTheJobSource: with an empty or relative home a
// repo-committed tools/syft-<v>/syft must not be taken for the installed syft.
func TestRunNeverRunsAToolPlantedInTheJobSource(t *testing.T) {
	src := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	planted := filepath.Join(src, "tools", "syft-"+pinV, "syft")
	if err := os.MkdirAll(filepath.Dir(planted), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planted, []byte("#!/bin/sh\ntouch "+marker+"\necho 'Version: "+pinV+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(src)
	t.Setenv("NSELF_CI_HOME", "")
	t.Setenv("HOME", t.TempDir()) // the default home is empty
	t.Setenv("PATH", t.TempDir())
	for _, home := range []string{"", "tools/..", ".", "rel-home"} {
		in := gsbom.Input{
			Job:     model.Job{Kind: "release", Run: []string{gsbom.CheckID}},
			Runner:  sbom.ExecRunner{Executor: &exec.Executor{}, Home: t.TempDir(), SourceDir: src},
			Home:    home,
			Pin:     sbom.Pin{Version: pinV},
			Targets: []sbom.Target{{Name: "cli", Path: "tools", Kind: sbom.SourceDir}},
		}
		got := gsbom.Run(context.Background(), in).Check
		if got.Result != "error" || !got.Required {
			t.Errorf("home %q: %+v", home, got)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("the planted tool was executed")
	}
}
