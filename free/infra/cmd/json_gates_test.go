// Purpose: prove the consent gates of apply, destroy and server destroy
// (CANON-12, DEPL D9) hold in both modes and in the documented order: a refusal
// leaves terraform and the Hetzner delete call unreached, exits with the same
// status as before in human mode, and exits 4 with an E403 envelope under --json.
//
// Inputs: the in-process harness; the stub tofu logs every call to STUB_LOG, the
// recording client counts delete and snapshot calls.
//
// Outputs: test results.
//
// Constraints: no network, no cloud credentials, no spend.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/output"
)

func TestTerraformGatesRefuseBeforeTerraform(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		tty       bool
		humanExit int
		humanErr  string // text expected on stderr in human mode
	}{
		{"apply without the spend flag", []string{"apply", "--provider", "hetzner", "--domain", tfDomain, "--auto-approve"}, true, 4, "will incur cloud spend"},
		{"apply without a terminal", []string{"apply", "--provider", "hetzner", "--domain", tfDomain, consent, "--auto-approve"}, false, 4, "requires a TTY"},
		{"destroy without the spend flag", []string{"destroy", "--provider", "hetzner", "--auto-approve"}, true, 4, "will incur cloud spend"},
		{"destroy without a terminal", []string{"destroy", "--provider", "hetzner", consent, "--auto-approve"}, false, 4, "requires a TTY"},
		{"destroy with spend and terminal but no --auto-approve", []string{"destroy", "--provider", "hetzner", consent}, true, 1, "requires --auto-approve to proceed"},
		{"destroy of an unshipped provider", []string{"destroy", "--provider", "aws", consent, "--auto-approve"}, true, 1, "not shipped (DEF-005)"},
		{"destroy of an unknown provider", []string{"destroy", "--provider", "nope", consent, "--auto-approve"}, true, 1, "unknown provider"},
		{"destroy with no provider", []string{"destroy", consent, "--auto-approve"}, true, 1, "--provider is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name+" (human)", func(t *testing.T) {
			r := runCLI(t, cliEnv{Tofu: "ok", TTY: tc.tty}, tc.args...)
			if r.TofuLog != "" {
				t.Fatalf("terraform was invoked on a refused run: %q", r.TofuLog)
			}
			if r.Exit != tc.humanExit {
				t.Errorf("exit = %d, want %d\nstderr:\n%s", r.Exit, tc.humanExit, r.Stderr)
			}
			if !strings.Contains(r.Stderr, tc.humanErr) {
				t.Errorf("stderr lacks %q:\n%s", tc.humanErr, r.Stderr)
			}
			if r.Stdout != "" {
				t.Errorf("a refusal printed on stdout: %q", r.Stdout)
			}
		})
		t.Run(tc.name+" (json)", func(t *testing.T) {
			r := runCLI(t, cliEnv{Tofu: "ok", TTY: tc.tty}, append(append([]string{}, tc.args...), "--json")...)
			if r.TofuLog != "" {
				t.Fatalf("terraform was invoked on a refused run: %q", r.TofuLog)
			}
			if err := output.CheckEnvelope([]byte(r.Stdout)); err != nil {
				t.Fatalf("not one envelope: %v\n%s", err, r.Stdout)
			}
			wantExit := output.ExitUser
			if tc.humanExit == 4 || strings.Contains(tc.name, "--auto-approve") {
				wantExit = output.ExitDestructiveBlocked
			}
			if r.Exit != wantExit {
				t.Errorf("exit = %d, want %d\n%s", r.Exit, wantExit, r.Stdout)
			}
		})
	}
}

// A consent flag typed after the others does not skip the terminal check: every
// gate must pass, in order, for terraform to be reached.
func TestTerraformRunsOnlyAfterEveryGate(t *testing.T) {
	r := runCLI(t, cliEnv{Tofu: "ok", TTY: true}, "destroy", "--provider", "hetzner", consent, "--auto-approve")
	if r.Exit != 0 || !strings.Contains(r.TofuLog, "destroy") {
		t.Fatalf("a fully confirmed destroy must run terraform: exit=%d log=%q stderr=%s", r.Exit, r.TofuLog, r.Stderr)
	}
}

func TestServerDestroyRefusalExitsAndNeverDeletes(t *testing.T) {
	for _, js := range []bool{false, true} {
		rc := newRecording()
		args := []string{"server", "destroy", "--id", "1"}
		want := 1
		if js {
			args = append(args, "--json")
			want = output.ExitDestructiveBlocked
		}
		r := runCLI(t, cliEnv{Client: rc}, args...)
		if r.Exit != want {
			t.Errorf("json=%v: exit = %d, want %d\n%s%s", js, r.Exit, want, r.Stdout, r.Stderr)
		}
		if rc.deletes != 0 || rc.snapshots != 0 || rc.ipWrites != 0 {
			t.Errorf("json=%v: a refused destroy touched the provider: deletes=%d snapshots=%d ipWrites=%d", js, rc.deletes, rc.snapshots, rc.ipWrites)
		}
	}
}

// The production check, not the test override: /dev/null (what exec gives a
// child by default) and a regular file are not terminals.
func TestProductionTTYCheckRejectsNullAndFiles(t *testing.T) {
	old := os.Stdin
	t.Cleanup(func() { os.Stdin = old })

	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	reg, err := os.Create(filepath.Join(t.TempDir(), "stdin"))
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	for name, f := range map[string]*os.File{"/dev/null": null, "regular file": reg} {
		os.Stdin = f
		if stdinIsTTY() {
			t.Errorf("stdin on %s counted as a terminal", name)
		}
		if isTerminal(f) {
			t.Errorf("isTerminal(%s) = true", name)
		}
	}
}

// With every consent flag but a stdin that is not a terminal (the production
// check, stdin on /dev/null), apply and destroy refuse: E403, exit 4, and the
// stub terraform is never run.
func TestApplyDestroyRefuseNonTTYStdin(t *testing.T) {
	old := os.Stdin
	t.Cleanup(func() { os.Stdin = old })
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	os.Stdin = null

	for _, args := range [][]string{
		{"apply", "--provider", "hetzner", "--domain", tfDomain, consent, "--auto-approve", "--json"},
		{"destroy", "--provider", "hetzner", consent, "--auto-approve", "--json"},
	} {
		r := runCLI(t, cliEnv{Tofu: "ok", RealTTY: true}, args...)
		if r.Exit != output.ExitDestructiveBlocked {
			t.Errorf("%s: exit = %d, want 4\n%s", args[0], r.Exit, r.Stdout)
		}
		if !strings.Contains(r.Stdout, `"code": "E403"`) {
			t.Errorf("%s: no E403 envelope:\n%s", args[0], r.Stdout)
		}
		if r.TofuLog != "" {
			t.Errorf("%s: terraform was invoked: %q", args[0], r.TofuLog)
		}
	}
}
