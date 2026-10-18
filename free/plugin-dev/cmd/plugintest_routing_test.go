package main

// Purpose: TestPluginTestFailureRouting, the exit class of `test` by cause: a
// child that ran and failed is a user failure (E773, exit 1), a child that could
// not be launched (no go, no nself on PATH) is an infrastructure failure (E776,
// exit 2). Debt D-0927.
// Inputs:  stub tools on PATH, built with stubBin; an empty PATH for "missing".
// Outputs: test results.
// Constraints: no network, no real go or nself. A failing child's stdout and
// stderr sentinels must reach stderr only: stdout holds the one envelope.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginTestFailureRouting(t *testing.T) {
	const outMark, errMark = "CHILD-STDOUT-SENTINEL", "CHILD-STDERR-SENTINEL"
	noisy := "echo " + outMark + "\necho " + errMark + " >&2\nexit 1"
	cases := []struct {
		name  string
		args  []string
		setup func(t *testing.T, root string)
		code  int
		ecode string
		class string
	}{
		{name: "go-ran-and-failed", args: []string{"test", "rt", "--phase", "unit", "--host", "--json"}, code: 1, ecode: CodeTestsFailed, class: "user",
			setup: func(t *testing.T, root string) {
				pluginDir(t, root, "rt")
				stubs(t, map[string]string{"go": noisy})
			}},
		{name: "go-missing", args: []string{"test", "rt", "--phase", "unit", "--host", "--json"}, code: 2, ecode: CodeStepFailed, class: "infra",
			setup: func(t *testing.T, root string) {
				pluginDir(t, root, "rt")
				onlyStubs(t)
			}},
		{name: "nself-ran-and-failed", args: []string{"test", "rt", "--phase", "smoke", "--no-cleanup", "--json"}, code: 1, ecode: CodeTestsFailed, class: "user",
			setup: func(t *testing.T, root string) {
				pluginDir(t, root, "rt")
				stubs(t, map[string]string{"nself": noisy})
			}},
		{name: "nself-missing", args: []string{"test", "rt", "--phase", "smoke", "--no-cleanup", "--json"}, code: 2, ecode: CodeStepFailed, class: "infra",
			setup: func(t *testing.T, root string) {
				pluginDir(t, root, "rt")
				onlyStubs(t)
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("HOME", root)
			t.Setenv("NO_COLOR", "1")
			if err := os.MkdirAll(filepath.Join(root, "work"), 0o750); err != nil {
				t.Fatal(err)
			}
			t.Chdir(filepath.Join(root, "work"))
			c.setup(t, root)
			var code int
			stdout, stderr := captureBytes(t, func() { code = run(c.args) })
			if code != c.code {
				t.Fatalf("exit status %d, want %d (stderr %q)", code, c.code, stderr)
			}
			checkEnvelopeBytes(t, stdout, c.code)
			var d struct {
				Error struct{ Code, Class string } `json:"error"`
			}
			if err := json.Unmarshal(stdout, &d); err != nil {
				t.Fatal(err)
			}
			if d.Error.Code != c.ecode || d.Error.Class != c.class {
				t.Errorf("error %s/%s, want %s/%s: %s", d.Error.Code, d.Error.Class, c.ecode, c.class, stdout)
			}
			if strings.Contains(string(stdout), "SENTINEL") {
				t.Errorf("child output leaked into the envelope: %s", stdout)
			}
			if strings.Contains(c.name, "ran-and-failed") &&
				(!strings.Contains(stderr, outMark) || !strings.Contains(stderr, errMark)) {
				t.Errorf("child output missing from stderr: %q", stderr)
			}
		})
	}
}
