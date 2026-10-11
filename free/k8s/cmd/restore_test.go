// Purpose: black-box tests of `nself-k8s restore`: the --yes gate, the integrity
// failures (tampered dump, fingerprint mismatch) and the verified success.
//
// Inputs: runK8s and the fixtures of backup_test.go.
//
// Outputs: test results.
//
// Constraints: restore reports success only after equal fingerprints; every refusal
// asserts that the stub kubectl was never called.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/output"
)

// madeBackup runs a backup through the binary and returns the dump path.
func madeBackup(t *testing.T, dir string) string {
	t.Helper()
	r := runK8s(t, k8sRun{Dir: dir, Mode: "backup", Dump: "PGDMP-synthetic", Fps: fpLines}, "backup", "--json")
	d := wantSuccess(t, r.cliResult, "k8s backup")
	return d.Data["file"].(string)
}

func TestRestoreWithoutYesIsRefusedAndRunsNothing(t *testing.T) {
	dir := project(t)
	file := madeBackup(t, dir)
	for _, args := range [][]string{{"restore", file}, {"restore", file, "--json"}} {
		// stdin is /dev/null: not a terminal, so no prompt could save it.
		r := runK8s(t, k8sRun{Dir: dir, Mode: "restore", PostFps: fpLines}, args...)
		if r.Exit != 4 {
			t.Errorf("%v: exit = %d, want 4\n%s%s", args, r.Exit, r.Stdout, r.Stderr)
		}
		if r.Calls != 0 {
			t.Errorf("%v: kubectl ran %d time(s)", args, r.Calls)
		}
		if strings.Contains(r.Stdout+r.Stderr, "Restored") {
			t.Errorf("%v: printed success", args)
		}
		if len(args) == 2 && !strings.Contains(r.Stderr, "Error: E403 ") {
			t.Errorf("human refusal lacks the code: %q", r.Stderr)
		}
		if len(args) == 3 {
			wantFailure(t, r.cliResult, "k8s restore", "E403", output.ClassDestructiveBlocked)
		}
	}
	// No kubectl on PATH at all: the refusal still comes first.
	r := runK8s(t, k8sRun{Dir: dir, NoKubectl: true}, "restore", file, "--json")
	wantFailure(t, r.cliResult, "k8s restore", "E403", output.ClassDestructiveBlocked)
}

func TestRestoreVerifiedSuccess(t *testing.T) {
	dir := project(t)
	file := madeBackup(t, dir)
	r := runK8s(t, k8sRun{Dir: dir, Mode: "restore", PostFps: fpLines}, "restore", file, "--yes", "--json")
	d := wantSuccess(t, r.cliResult, "k8s restore")
	if d.Data["verified"] != true || d.Data["file"] != file || len(d.Data["tables"].([]any)) != 3 {
		t.Errorf("data = %v", d.Data)
	}
	h := runK8s(t, k8sRun{Dir: dir, Mode: "restore", PostFps: fpLines}, "restore", file, "-y")
	if h.Exit != 0 || !strings.Contains(h.Stdout+h.Stderr, "3 tables verified") {
		t.Errorf("human: exit %d\n%s%s", h.Exit, h.Stdout, h.Stderr)
	}
}

func TestRestoreTamperedDumpExits2WithoutSuccess(t *testing.T) {
	dir := project(t)
	file := madeBackup(t, dir)
	b, _ := os.ReadFile(file)
	b[3] ^= 0x01
	if err := os.WriteFile(file, b, 0o600); err != nil {
		t.Fatal(err)
	}
	r := runK8s(t, k8sRun{Dir: dir, Mode: "restore", PostFps: fpLines}, "restore", file, "--yes", "--json")
	wantFailure(t, r.cliResult, "k8s restore", "E735", output.ClassInfra)
	if strings.Contains(r.Stdout, `"verified"`) {
		t.Error("stdout carries a success")
	}
	// A bad file never touches the cluster: not even the pod lookup.
	if r.Calls != 0 {
		t.Errorf("kubectl calls = %d, want 0", r.Calls)
	}
	// ... and it is reported before kubectl is looked up.
	nk := runK8s(t, k8sRun{Dir: dir, NoKubectl: true}, "restore", file, "--yes", "--json")
	wantFailure(t, nk.cliResult, "k8s restore", "E735", output.ClassInfra)
	h := runK8s(t, k8sRun{Dir: dir, Mode: "restore", PostFps: fpLines}, "restore", file, "--yes")
	if h.Exit != 2 || !strings.Contains(h.Stderr, "E735") || strings.Contains(h.Stdout+h.Stderr, "Restored") {
		t.Errorf("human: exit %d\n%s%s", h.Exit, h.Stdout, h.Stderr)
	}
}

func TestRestoreFingerprintMismatchExits2WithoutSuccess(t *testing.T) {
	dir := project(t)
	file := madeBackup(t, dir)
	changed := strings.Replace(fpLines, `"rows":1000`, `"rows":999`, 1)
	r := runK8s(t, k8sRun{Dir: dir, Mode: "restore", PostFps: changed}, "restore", file, "--yes", "--json")
	d := wantFailure(t, r.cliResult, "k8s restore", "E737", output.ClassInfra)
	if !strings.Contains(d.Error.Cause, "rows 1000 != 999") || strings.Contains(r.Stdout, `"verified"`) {
		t.Errorf("cause = %q\n%s", d.Error.Cause, r.Stdout)
	}
	h := runK8s(t, k8sRun{Dir: dir, Mode: "restore", PostFps: changed}, "restore", file, "--yes")
	if h.Exit != 2 || !strings.Contains(h.Stderr, "E737") || strings.Contains(h.Stdout+h.Stderr, "Restored") {
		t.Errorf("human: exit %d\n%s%s", h.Exit, h.Stdout, h.Stderr)
	}
}

func TestRestoreRefusesDumpWithoutSidecar(t *testing.T) {
	dir := project(t)
	core := filepath.Join(dir, "core_full_20261011_040506.dump")
	writeFile(t, core, "PGDMP-made-by-nself-backup-create", 0o600)
	r := runK8s(t, k8sRun{Dir: dir, Mode: "restore", PostFps: fpLines}, "restore", core, "--yes", "--json")
	d := wantFailure(t, r.cliResult, "k8s restore", "E734", output.ClassUser)
	if !strings.Contains(d.Error.Message, "no metadata") {
		t.Errorf("message = %q", d.Error.Message)
	}
	if r.Calls != 0 {
		t.Errorf("kubectl calls = %d, want 0", r.Calls)
	}
	nk := runK8s(t, k8sRun{Dir: dir, NoKubectl: true}, "restore", core, "--yes", "--json")
	wantFailure(t, nk.cliResult, "k8s restore", "E734", output.ClassUser)
}

func TestRestorePgRestoreErrorIsE736(t *testing.T) {
	dir := project(t)
	file := madeBackup(t, dir)
	r := runK8s(t, k8sRun{Dir: dir, Mode: "restore", PostFps: fpLines, RestoreErr: "pg_restore: error: could not execute query: ERROR:  boom"}, "restore", file, "--yes", "--json")
	wantFailure(t, r.cliResult, "k8s restore", "E736", output.ClassInfra)
}

func TestRestoreUsageErrors(t *testing.T) {
	dir := project(t)
	r := runK8s(t, k8sRun{Dir: dir}, "restore", "--json")
	wantFailure(t, r.cliResult, "k8s restore", "E401", output.ClassUser)
	if r.Calls != 0 {
		t.Error("kubectl ran")
	}
}

func TestRestoreWarnsWhenTargetDiffersFromBackup(t *testing.T) {
	dir := project(t)
	file := madeBackup(t, dir) // taken from release "nself"
	r := runK8s(t, k8sRun{Dir: dir, Mode: "restore", PostFps: fpLines}, "restore", file, "--yes", "--json", "--release", "other", "--namespace", "ns9")
	d := wantSuccess(t, r.cliResult, "k8s restore")
	if len(d.Meta.Warnings) != 1 || !strings.Contains(d.Meta.Warnings[0], `"nself"`) || !strings.Contains(d.Meta.Warnings[0], `"other"`) {
		t.Errorf("warnings = %q", d.Meta.Warnings)
	}
	// The namespace comes from the pod itself, not the flag: a different pod namespace warns.
	other := strings.Replace(onePod, "pod-ns", "other-ns", 1)
	n := runK8s(t, k8sRun{Dir: dir, Mode: "restore", PostFps: fpLines, Pods: other}, "restore", file, "--yes", "--json")
	nd := wantSuccess(t, n.cliResult, "k8s restore")
	if len(nd.Meta.Warnings) != 1 || !strings.Contains(nd.Meta.Warnings[0], `"pod-ns"`) || !strings.Contains(nd.Meta.Warnings[0], `"other-ns"`) {
		t.Errorf("namespace warnings = %q", nd.Meta.Warnings)
	}
	h := runK8s(t, k8sRun{Dir: dir, Mode: "restore", PostFps: fpLines}, "restore", file, "--yes", "--release", "other")
	if h.Exit != 0 {
		t.Errorf("a differing target must not refuse: exit %d\n%s%s", h.Exit, h.Stdout, h.Stderr)
	}
}

func TestRestoreDatabaseTextNeverReachesTheEnvelope(t *testing.T) {
	const secret = "row-secret-value-9"
	dir := project(t)
	file := madeBackup(t, dir)
	errLine := "pg_restore: error: COPY failed for table \"users\": ERROR:  invalid input syntax for type uuid: '" + secret + "'"
	j := runK8s(t, k8sRun{Dir: dir, Mode: "restore", PostFps: fpLines, RestoreErr: errLine}, "restore", file, "--yes", "--json")
	d := wantFailure(t, j.cliResult, "k8s restore", "E736", output.ClassInfra)
	if strings.Contains(j.Stdout, secret) || strings.Contains(j.Stderr, secret) || strings.Contains(d.Error.Cause, "COPY") {
		t.Errorf("database text in the JSON run:\n%s\n%s", j.Stdout, j.Stderr)
	}
	h := runK8s(t, k8sRun{Dir: dir, Mode: "restore", PostFps: fpLines, RestoreErr: errLine}, "restore", file, "--yes")
	if h.Exit != 1 || strings.Contains(h.Stdout+h.Stderr, secret) || strings.Contains(h.Stdout+h.Stderr, "users") {
		t.Errorf("human: exit %d\n%s%s", h.Exit, h.Stdout, h.Stderr)
	}
	if !strings.Contains(h.Stderr, "first error line:") || !strings.Contains(h.Stderr, "…") || strings.Contains(h.Stderr, "DETAIL") {
		t.Errorf("human stderr lacks the masked first error line: %q", h.Stderr)
	}
}
