// Purpose: black-box harness for the nself-k8s binary: TestMain builds it
// once, runCLI execs it with a stub helm (and docker) on PATH.
//
// Inputs: ../testdata/full (recorded `docker compose config` JSON and project
// manifests), the binary built from this package.
//
// Outputs: helpers (runCLI, newProject, writeGenerated) used by the golden and
// the JSON-contract tests.
//
// Constraints: no network, no cluster, no real helm or docker. The stub helm
// records its argv to a file, so a test can prove that a refused command ran
// no helm at all. stdin of the process is /dev/null unless a test sets it.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var cliBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "nself-k8s-cli-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	cliBin = filepath.Join(dir, "nself-k8s")
	build := exec.Command("go", "build", "-o", cliBin, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building nself-k8s: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(2)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// stubHelm records argv (one block per call, blocks end with "--"), drains
// stdin, then behaves by STUB_HELM: "fail" exits 1 with a message on stderr;
// "notfound" exits 1 with helm's release-not-found message;
// status prints the file STUB_STATUS_FILE; anything else prints one line on
// stdout and one on stderr and exits 0.
const stubHelm = `#!/bin/sh
if [ -n "$STUB_ARGV" ]; then
  { printf '%s\n' "$@"; echo '--'; } >> "$STUB_ARGV"
fi
cat >/dev/null
if [ "$STUB_HELM" = "fail" ]; then
  echo "stub helm: boom" >&2
  exit 1
fi
if [ "$STUB_HELM" = "notfound" ]; then
  echo "Error: release: not found" >&2
  exit 1
fi
if [ "$1" = "status" ]; then
  cat "$STUB_STATUS_FILE"
  exit 0
fi
echo "STUB-HELM $1 stdout"
echo "STUB-HELM $1 stderr" >&2
exit 0
`

// stubDocker prints the recorded compose JSON, or fails when STUB_DOCKER=fail.
const stubDocker = `#!/bin/sh
if [ "$STUB_DOCKER" = "fail" ]; then
  echo "stub docker: no daemon" >&2
  exit 1
fi
cat "$STUB_COMPOSE_JSON"
`

// helmStatusJSON is a release document with a secret-looking config block that
// status must never print.
const helmStatusJSON = `{"name":"nself","namespace":"default","version":3,"chart":{"metadata":{"version":"0.1.0"}},"info":{"status":"deployed"},"config":{"secret":"SHOULD-NEVER-PRINT"},"manifest":"kind: Secret"}`

// cliRun configures one invocation.
type cliRun struct {
	Helm      string // "" (none on PATH), "ok", "fail", "notfound"
	Docker    string // "" (none on PATH), "ok", "fail"
	StatusDoc string // helm status output; default helmStatusJSON
	Env       []string
	Dir       string // working directory
	Stdin     io.Reader
}

// cliResult is what the process did.
type cliResult struct {
	Stdout, Stderr string
	Exit           int
	ArgvFile       string // the stub helm argv record; may not exist
}

// runCLI execs the built binary with args.
func runCLI(t *testing.T, o cliRun, args ...string) cliResult {
	t.Helper()
	stubs := t.TempDir()
	argv := filepath.Join(stubs, "helm.argv")
	status := filepath.Join(stubs, "status.json")
	doc := o.StatusDoc
	if doc == "" {
		doc = helmStatusJSON
	}
	writeFile(t, status, doc, 0o644)
	if o.Helm != "" {
		writeFile(t, filepath.Join(stubs, "helm"), stubHelm, 0o755)
	}
	if o.Docker != "" {
		writeFile(t, filepath.Join(stubs, "docker"), stubDocker, 0o755)
	}
	compose, err := filepath.Abs(filepath.Join("..", "testdata", "full", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	env := []string{
		"PATH=" + stubs + ":/usr/bin:/bin",
		"HOME=" + stubs,
		"STUB_ARGV=" + argv,
		"STUB_STATUS_FILE=" + status,
		"STUB_COMPOSE_JSON=" + compose,
	}
	if o.Helm == "fail" || o.Helm == "notfound" {
		env = append(env, "STUB_HELM="+o.Helm)
	}
	if o.Docker == "fail" {
		env = append(env, "STUB_DOCKER=fail")
	}
	cmd := exec.Command(cliBin, args...)
	cmd.Env = append(env, o.Env...)
	cmd.Dir = o.Dir
	cmd.Stdin = o.Stdin
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err = cmd.Run()
	res := cliResult{Stdout: so.String(), Stderr: se.String(), ArgvFile: argv}
	if ee, ok := err.(*exec.ExitError); ok {
		res.Exit = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running %s: %v", cliBin, err)
	}
	return res
}

func writeFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

// newProject copies the compose manifests of testdata/full into a temp project
// (no generated values yet) and returns its absolute path.
func newProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range []string{".nself/compose-files.txt", ".nself/compose-env-files.txt", ".nself/generated/routes.json"} {
		b, err := os.ReadFile(filepath.Join("..", "testdata", "full", f))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(dir, f), string(b), 0o644)
	}
	return dir
}

// writeGenerated writes minimal chart values so install/upgrade pass the check.
func writeGenerated(t *testing.T, dir string) {
	t.Helper()
	gen := filepath.Join(dir, ".nself", "generated", "k8s")
	writeFile(t, filepath.Join(gen, "values.yaml"), "ingress:\n  rules:\n    - {name: h, host: api.example.test, service: hasura, port: 8080, scheme: http}\n", 0o644)
	writeFile(t, filepath.Join(gen, "secrets.yaml"), "secrets: {}\n", 0o600)
}

// argvCalls reads the stub helm record: one []string per helm call.
func argvCalls(t *testing.T, file string) [][]string {
	t.Helper()
	b, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	var cur []string
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if l == "--" {
			calls = append(calls, cur)
			cur = nil
			continue
		}
		cur = append(cur, l)
	}
	return calls
}
