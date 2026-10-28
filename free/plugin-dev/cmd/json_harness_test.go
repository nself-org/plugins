package main

// Purpose: the harness behind TestJSONEnvelope: run one plugin-dev invocation
// in-process with stdout and stderr captured, check that stdout holds exactly
// one v1 envelope (output.CheckEnvelope), that the exit status equals the
// envelope's class status, and compare the envelope with its golden file.
// Inputs:  a jsonCase (argv, setup, expected status) and a fresh sandbox root.
// Outputs: test results. `go test ./cmd -run TestJSONEnvelope -update` rewrites
// testdata/json/*.json.
// Constraints: no network, no real nself, go, dlv, bash or curl: the external
// tools are stub scripts on PATH. Machine paths are replaced by <TMP> so the
// goldens hold synthetic values only.

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/output"
)

var update = flag.Bool("update", false, "rewrite testdata/json/*.json")

// jsonCase is one --json invocation.
type jsonCase struct {
	name  string                          // golden file name
	args  []string                        // argv after the binary name
	setup func(t *testing.T, root string) // prepare files and stubs under root
	code  int                             // expected exit status
}

// envDoc is the decoded envelope.
type envDoc struct {
	Command string         `json:"command"`
	Data    map[string]any `json:"data"`
	Error   *struct {
		Code     string `json:"code"`
		ExitCode int    `json:"exit_code"`
	} `json:"error"`
}

// runCase runs c in a fresh sandbox and checks it against its golden.
func runCase(t *testing.T, c jsonCase) {
	golden, err := filepath.Abs(filepath.Join("..", "testdata", "json", c.name+".json")) // before the chdir
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	work := filepath.Join(root, "work")
	for _, d := range []string{home, work} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TMPDIR", root)
	t.Chdir(work)
	if c.setup != nil {
		c.setup(t, root)
	}
	var code int
	stdout, stderr := captureBytes(t, func() { code = run(c.args) })
	if code != c.code {
		t.Fatalf("exit status %d, want %d (stderr %q)", code, c.code, stderr)
	}
	if c.name == "test-unit-ok" || c.name == "test-unit-fails" || c.name == "test-unit-docker-launch" {
		for _, sentinel := range []string{"child-stdout", "child-stderr"} {
			if strings.Contains(string(stdout), sentinel) || !strings.Contains(stderr, sentinel) {
				t.Errorf("child output %q must appear only on stderr (stdout %q, stderr %q)", sentinel, stdout, stderr)
			}
		}
	}
	checkEnvelopeBytes(t, stdout, c.code)
	got := normalize(stdout, root)
	path := golden
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden %s: %v (run with -update)", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden %s differs\n got: %s\nwant: %s", path, got, want)
	}
}

// captureBytes runs fn with os.Stdout and os.Stderr on pipes and returns both.
func captureBytes(t *testing.T, fn func()) ([]byte, string) {
	t.Helper()
	ro, wo, _ := os.Pipe()
	re, we, _ := os.Pipe()
	so, se := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wo, we
	done := make(chan []byte)
	go func() { b, _ := io.ReadAll(ro); done <- b }()
	errDone := make(chan []byte)
	go func() { b, _ := io.ReadAll(re); errDone <- b }()
	fn()
	os.Stdout, os.Stderr = so, se
	_ = wo.Close()
	_ = we.Close()
	return <-done, string(<-errDone)
}

// checkEnvelopeBytes asserts stdout is exactly one valid v1 envelope and that
// code matches the error's exit_code (0 for a data envelope).
func checkEnvelopeBytes(t *testing.T, stdout []byte, code int) {
	t.Helper()
	if err := output.CheckEnvelope(stdout); err != nil {
		t.Fatalf("CheckEnvelope: %v\nstdout: %s", err, stdout)
	}
	dec := json.NewDecoder(bytes.NewReader(stdout))
	var d envDoc
	if err := dec.Decode(&d); err != nil {
		t.Fatal(err)
	}
	if dec.More() {
		t.Fatalf("stdout carries more than one document: %s", stdout)
	}
	if !strings.HasPrefix(d.Command, "plugin-dev") {
		t.Errorf("command %q", d.Command)
	}
	switch {
	case code == 0 && (d.Error != nil || d.Data == nil):
		t.Errorf("exit 0 needs a data envelope: %s", stdout)
	case code != 0 && (d.Error == nil || d.Error.ExitCode != code):
		t.Errorf("exit %d needs an error envelope with that exit_code: %s", code, stdout)
	}
}

// normalize replaces the sandbox root (and its symlink-resolved form) by <TMP>.
func normalize(b []byte, root string) []byte {
	s := string(b)
	if r, err := filepath.EvalSymlinks(root); err == nil {
		s = strings.ReplaceAll(s, r, "<TMP>")
	}
	return []byte(strings.ReplaceAll(s, root, "<TMP>"))
}

// pluginDir makes <root>/work/<name> a directory that holds a plugin.yaml.
func pluginDir(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, "work", name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte("name: "+name+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}
