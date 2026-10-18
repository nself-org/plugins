package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

const digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// world is a fake host: which tools exist, whether gh is signed in, and what
// `git check-ignore` says.
type world struct {
	noDocker, dockerDown, noGo, noGH, ignored bool
	noRemote                                  bool
}

func (w world) capture(_ context.Context, argv ...string) (string, int, error) {
	joined := strings.Join(argv, " ")
	switch {
	case argv[0] == "go":
		if w.noGo {
			return "", 127, nil
		}
		return "go version go1.24.0 darwin/arm64\n", 0, nil
	case argv[0] == "docker":
		if w.noDocker {
			return "", 0, errors.New("not found")
		}
		if w.dockerDown {
			return "Cannot connect to the Docker daemon\n", 1, nil
		}
		return "27.1.0\n", 0, nil
	case argv[0] == "gh":
		if w.noGH {
			return "not logged in\n", 1, nil
		}
		return "ok\n", 0, nil
	case strings.Contains(joined, "remote get-url"):
		if w.noRemote {
			return "", 2, nil
		}
		return "git@github.com:example/demo.git\n", 0, nil
	case strings.Contains(joined, "check-ignore"):
		if w.ignored {
			return "", 0, nil
		}
		return "", 1, nil
	}
	return "", 0, nil
}

func (w world) deps(t *testing.T) Deps {
	t.Helper()
	home := t.TempDir()
	return Deps{
		Getenv:    func(k string) string { return map[string]string{"NSELF_CI_HOME": home}[k] },
		Getwd:     func() (string, error) { return "", errors.New("no cwd") },
		UserHome:  func() (string, error) { return home, nil },
		Capture:   w.capture,
		OpenStore: DefaultDeps().OpenStore,
	}
}

// repo makes a Go repository root, with an optional .nself/ci.yaml.
func repo(t *testing.T, config string) string {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module example.test/demo\n\ngo 1.24\n")
	if config != "" {
		write(t, filepath.Join(root, ".nself", "ci.yaml"), config)
	}
	return root
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func byID(checks []Check, id string) []Check {
	var out []Check
	for _, c := range checks {
		if c.ID == id {
			out = append(out, c)
		}
	}
	return out
}

func one(t *testing.T, checks []Check, id string) Check {
	t.Helper()
	got := byID(checks, id)
	if len(got) != 1 {
		t.Fatalf("%s: want 1 check, got %d in %+v", id, len(got), checks)
	}
	return got[0]
}

// resetSections empties the registry for one test.
func resetSections(t *testing.T) {
	t.Helper()
	sectionMu.Lock()
	saved := sections
	sections = nil
	sectionMu.Unlock()
	t.Cleanup(func() { sectionMu.Lock(); sections = saved; sectionMu.Unlock() })
}

func TestDoctorExitCodes(t *testing.T) {
	resetSections(t)
	for _, tc := range []struct {
		name   string
		checks []Check
		want   int
	}{
		{"none", nil, 0},
		{"clean", []Check{{Status: Pass}}, 0},
		{"warn", []Check{{Status: Pass}, {Status: Warn}}, 12},
		{"fail", []Check{{Status: Warn}, {Status: Fail}}, 10},
	} {
		if got := ExitCode(tc.checks); got != tc.want {
			t.Errorf("%s: ExitCode = %d, want %d", tc.name, got, tc.want)
		}
	}

	run := func(t *testing.T, w world, root string) (int, reportData) {
		t.Helper()
		var out, errOut bytes.Buffer
		code := Run([]string{"--json", "--repo", root}, &out, &errOut, w.deps(t))
		var env struct {
			Data reportData `json:"data"`
		}
		if err := json.Unmarshal(out.Bytes(), &env); err != nil {
			t.Fatalf("not an envelope: %v\n%s%s", err, out.String(), errOut.String())
		}
		return code, env.Data
	}

	t.Run("clean host exits 0", func(t *testing.T) {
		code, data := run(t, world{}, repo(t, ""))
		if code != 0 {
			t.Fatalf("exit %d, checks %+v", code, data.Checks)
		}
		for _, c := range data.Checks {
			if c.ID == "" || c.Status == "" {
				t.Errorf("check without id or status: %+v", c)
			}
		}
	})
	t.Run("warnings only exit 12", func(t *testing.T) {
		code, data := run(t, world{noDocker: true}, repo(t, ""))
		if code != 12 || data.Summary.Fail != 0 || data.Summary.Warn == 0 {
			t.Fatalf("exit %d, summary %+v", code, data.Summary)
		}
	})
	t.Run("a failure exits 10", func(t *testing.T) {
		root := repo(t, "")
		write(t, filepath.Join(root, ".ci.yaml"), "checks: []\n")
		code, data := run(t, world{}, root)
		if code != 10 || data.Summary.Fail == 0 {
			t.Fatalf("exit %d, summary %+v", code, data.Summary)
		}
	})
	t.Run("human output prints the fix", func(t *testing.T) {
		var out bytes.Buffer
		code := Run([]string{"--repo", repo(t, "")}, &out, &out, world{noDocker: true}.deps(t))
		if code != 12 || !strings.Contains(out.String(), "CI-D002") || !strings.Contains(out.String(), "fix:") {
			t.Fatalf("exit %d\n%s", code, out.String())
		}
	})
	t.Run("usage error exits 1", func(t *testing.T) {
		var out bytes.Buffer
		if code := Run([]string{"extra"}, &out, &out, world{}.deps(t)); code != 1 {
			t.Fatalf("exit %d", code)
		}
	})
}

func TestDockerAndToolchainSeverity(t *testing.T) {
	jobs := "version: 1\njobs:\n  unit:\n    kind: test\n    run: [go, test]\n  boxed:\n    kind: test\n    run: [make, test]\n" +
		"    isolation: container\n    container:\n      image: alpine@" + digest + "\n"
	root := repo(t, jobs)
	w := world{noDocker: true, noGo: true}
	checks := RunLocal(context.Background(), root, w.deps(t))
	docker := one(t, checks, "CI-D002")
	if docker.Status != Fail || !strings.Contains(docker.Message, "boxed") {
		t.Errorf("docker with a container job: %+v", docker)
	}
	goChk := one(t, checks, "CI-D001")
	if goChk.Status != Fail || !strings.Contains(goChk.Message, "unit") || goChk.Subject != "go" {
		t.Errorf("go with a go job: %+v", goChk)
	}

	// The same host with no config: both are only warnings.
	checks = RunLocal(context.Background(), repo(t, ""), w.deps(t))
	if c := one(t, checks, "CI-D002"); c.Status != Warn || c.Remediation == "" {
		t.Errorf("docker without a job: %+v", c)
	}
	if c := one(t, checks, "CI-D001"); c.Status != Warn {
		t.Errorf("go without a job: %+v", c)
	}
	// A daemon that is down differs from a missing client.
	checks = RunLocal(context.Background(), repo(t, ""), world{dockerDown: true}.deps(t))
	if c := one(t, checks, "CI-D002"); c.Status != Warn || !strings.Contains(c.Message, "not reachable") {
		t.Errorf("docker down: %+v", c)
	}
}

func TestStatusPostingReasons(t *testing.T) {
	for _, tc := range []struct {
		name string
		w    world
		env  string
		want Status
		text string
	}{
		{"on", world{}, "", Pass, "status posting on (example/demo)"},
		{"no auth", world{noGH: true}, "", Warn, "status posting off: gh not authenticated"},
		{"no remote", world{noRemote: true}, "", Warn, "status posting off: no GitHub remote"},
		{"opted out", world{noGH: true}, "1", Pass, "status posting off: --no-status"},
	} {
		d := tc.w.deps(t)
		getenv := d.Getenv
		d.Getenv = func(k string) string {
			if k == "NSELF_CI_SKIP_STATUS" {
				return tc.env
			}
			return getenv(k)
		}
		c := one(t, RunLocal(context.Background(), repo(t, ""), d), "CI-D003")
		if c.Status != tc.want || !strings.Contains(c.Message, tc.text) {
			t.Errorf("%s: %+v", tc.name, c)
		}
	}
}

func TestDistinctRepositoryAndStoreChecks(t *testing.T) {
	ctx := context.Background()
	t.Run("invalid config", func(t *testing.T) {
		checks := RunLocal(ctx, repo(t, "version: 1\nzzz: 1\n"), world{}.deps(t))
		if c := one(t, checks, "CI-D006"); c.Status != Fail || !strings.Contains(c.Detail, "E620") {
			t.Errorf("%+v", c)
		}
	})
	t.Run("valid config", func(t *testing.T) {
		checks := RunLocal(ctx, repo(t, "version: 1\njobs:\n  a:\n    kind: test\n    run: [go, test]\n"), world{}.deps(t))
		if c := one(t, checks, "CI-D006"); c.Status != Pass {
			t.Errorf("%+v", c)
		}
	})
	t.Run("leftover ci yaml", func(t *testing.T) {
		root := repo(t, "")
		write(t, filepath.Join(root, "sub", ".ci.yaml"), "x: 1\n")
		c := one(t, RunLocal(ctx, root, world{}.deps(t)), "CI-D007")
		if c.Status != Fail || !strings.Contains(c.Message, "sub/.ci.yaml") || !strings.Contains(c.Remediation, "migrate") {
			t.Errorf("%+v", c)
		}
	})
	t.Run("git-ignored config", func(t *testing.T) {
		root := repo(t, "version: 1\n")
		c := one(t, RunLocal(ctx, root, world{ignored: true}.deps(t)), "CI-D008")
		if c.Status != Warn || !strings.Contains(c.Remediation, "!.nself/ci.yaml") {
			t.Errorf("%+v", c)
		}
	})
	t.Run("corrupt store", func(t *testing.T) {
		d := world{}.deps(t)
		path, _ := StorePath(d)
		write(t, path, strings.Repeat("this is not a database\n", 400))
		checks := RunLocal(ctx, repo(t, ""), d)
		if c := one(t, checks, "CI-D004"); c.Status != Fail || c.Detail == "" {
			t.Errorf("corrupt: %+v", c)
		}
		if c := one(t, checks, "CI-D005"); c.Status != Pass {
			t.Errorf("version of a corrupt store: %+v", c)
		}
	})
	t.Run("store newer than the binary", func(t *testing.T) {
		d := world{}.deps(t)
		path, _ := StorePath(d)
		write(t, path, "")
		d.OpenStore = func(string) error {
			return &store.Error{Code: "E606", Message: "database requires reader 9; upgrade nself-ci"}
		}
		checks := RunLocal(ctx, repo(t, ""), d)
		if c := one(t, checks, "CI-D005"); c.Status != Fail || !strings.Contains(c.Remediation, "Upgrade") {
			t.Errorf("newer: %+v", c)
		}
		if c := one(t, checks, "CI-D004"); c.Status != Pass {
			t.Errorf("open of a newer store: %+v", c)
		}
	})
	t.Run("healthy store and no store", func(t *testing.T) {
		d := world{}.deps(t)
		checks := RunLocal(ctx, repo(t, ""), d)
		if c := one(t, checks, "CI-D004"); c.Status != Pass || !strings.Contains(c.Message, "no store yet") {
			t.Errorf("fresh host: %+v", c)
		}
		path, _ := StorePath(d)
		if _, err := os.Stat(path); err == nil {
			t.Error("doctor created the state database")
		}
		st, err := store.Open(path, store.Options{})
		if err != nil {
			t.Fatal(err)
		}
		_ = st.Close()
		if c := one(t, RunLocal(ctx, repo(t, ""), d), "CI-D004"); c.Status != Pass || !strings.Contains(c.Message, "migrations are current") {
			t.Errorf("existing store: %+v", c)
		}
	})
}

func TestSections(t *testing.T) {
	resetSections(t)
	run := func(args ...string) (int, reportData) {
		var out, errOut bytes.Buffer
		code := Run(append([]string{"--json", "--repo", repo(t, "")}, args...), &out, &errOut, world{}.deps(t))
		var env struct {
			Data reportData `json:"data"`
		}
		if err := json.Unmarshal(out.Bytes(), &env); err != nil {
			t.Fatalf("%v\n%s", err, out.String())
		}
		return code, env.Data
	}

	code, data := run("--providers")
	if c := one(t, data.Checks, "CI-D091"); code != 0 || c.Message != "none registered" {
		t.Errorf("none registered: exit %d, %+v", code, c)
	}
	if code, data = run(); code != 0 || len(byID(data.Checks, "CI-D091")) != 0 || len(byID(data.Checks, "CI-D090")) != 0 {
		t.Errorf("sections appear without their flag: exit %d", code)
	}

	RegisterSection(Section{Name: "fake-providers", Selector: SelectProviders, Run: func(context.Context, Env) []Check {
		return []Check{{ID: "CI-DP001", Name: "fake provider", Status: Warn, Message: "provider quota low"}}
	}})
	RegisterSection(Section{Name: "fake-always", Selector: SelectAlways, Run: func(_ context.Context, env Env) []Check {
		return []Check{{ID: "CI-D500", Name: "always", Status: Pass, Message: env.Root}}
	}})
	code, data = run("--providers")
	if c := one(t, data.Checks, "CI-DP001"); code != 12 || c.Status != Warn {
		t.Errorf("registered section: exit %d, %+v", code, c)
	}
	if len(byID(data.Checks, "CI-D091")) != 0 || len(byID(data.Checks, "CI-D500")) != 1 {
		t.Errorf("providers report: %+v", data.Checks)
	}
	if _, data = run(); len(byID(data.Checks, "CI-DP001")) != 0 || len(byID(data.Checks, "CI-D500")) != 1 {
		t.Errorf("default report: %+v", data.Checks)
	}
}

func TestSectionRegistryRejectsMistakes(t *testing.T) {
	resetSections(t)
	ok := func(context.Context, Env) []Check { return nil }
	RegisterSection(Section{Name: "a", Run: ok})
	for name, s := range map[string]Section{
		"duplicate":   {Name: "a", Run: ok},
		"unknown":     {Name: "b", Selector: "cloud", Run: ok},
		"no function": {Name: "c"},
		"no name":     {Run: ok},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: no panic", name)
				}
			}()
			RegisterSection(s)
		}()
	}
}

func TestSectionBudget(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	s := Section{Name: "stuck", Run: func(context.Context, Env) []Check { <-block; return nil }}
	start := time.Now()
	checks := runOne(context.Background(), s, Env{}, 50*time.Millisecond)
	if len(checks) != 1 || checks[0].ID != "CI-D098" || checks[0].Status != Warn || time.Since(start) > 5*time.Second {
		t.Fatalf("%+v after %s", checks, time.Since(start))
	}
}
