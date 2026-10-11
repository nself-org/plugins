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
	"errors"
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
// child by default), /dev/zero, /dev/urandom, a pipe and a regular file are not
// terminals. (A positive control needs a pty and is not run here.)
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

	zero, err := os.Open("/dev/zero")
	if err != nil {
		t.Fatal(err)
	}
	defer zero.Close()
	urandom, err := os.Open("/dev/urandom")
	if err != nil {
		t.Fatal(err)
	}
	defer urandom.Close()
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()
	defer pw.Close()

	files := map[string]*os.File{"/dev/null": null, "regular file": reg, "/dev/zero": zero, "/dev/urandom": urandom, "pipe": pr}
	for name, f := range files {
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

// The order is fixed: with neither the spend flag nor a terminal, the refusal
// names the spend flag.
func TestSpendFlagIsCheckedBeforeTerminal(t *testing.T) {
	for _, verb := range []string{"apply", "destroy"} {
		args := []string{verb, "--provider", "hetzner", "--domain", tfDomain, "--auto-approve"}
		r := runCLI(t, cliEnv{Tofu: "ok", TTY: false}, args...)
		if r.Exit != output.ExitDestructiveBlocked || !strings.Contains(r.Stderr, "will incur cloud spend") || strings.Contains(r.Stderr, "TTY") {
			t.Errorf("%s human: exit=%d stderr=%q", verb, r.Exit, r.Stderr)
		}
		r = runCLI(t, cliEnv{Tofu: "ok", TTY: false}, append(args, "--json")...)
		if !strings.Contains(r.Stdout, "--i-accept-cloud-spend") || strings.Contains(r.Stdout, "interactive terminal") {
			t.Errorf("%s json: want the spend-flag refusal, got:\n%s", verb, r.Stdout)
		}
		if r.TofuLog != "" {
			t.Errorf("%s: terraform was invoked", verb)
		}
	}
}

// A mistyped flag must not put its value in the envelope or on stderr.
func TestUsageEnvelopeNeverEchoesFlagValues(t *testing.T) {
	const secret = "FAKE-SECRET-VALUE-123"
	cases := [][]string{
		{"server", "list", "--tokn=" + secret, "--json"},
		{"server", "list", "--tokn", secret, "--json"},
		{"server", "list", "-x" + secret, "--json"},
		{"server", "resize", "--id", secret, "--type", "cx41", "--json"},
		{"server", "resize", "--id=" + secret, "--json"},
		{"server", "resize", "--json", "--id"},
		{"plan", "--provider", secret, "--domain", tfDomain, "--json"},
		{"server", "provision", "--label", secret, "--name", "a", "--json"},
	}
	for _, args := range cases {
		r := runCLI(t, cliEnv{Client: fixtureClient()}, args...)
		if strings.Contains(r.Stdout, secret) || strings.Contains(r.Stderr, secret) {
			t.Errorf("%v leaked the value:\nstdout:\n%s\nstderr:\n%s", args, r.Stdout, r.Stderr)
		}
		if err := output.CheckEnvelope([]byte(r.Stdout)); err != nil || r.Exit != output.ExitUser {
			t.Errorf("%v: want one E401 envelope, exit 1: %v exit=%d\n%s", args, err, r.Exit, r.Stdout)
		}
	}
}

// A Hetzner or local failure leaves a redacted diagnostic on stderr (never in
// the envelope), and no token reaches either stream.
func TestHetznerFailureDiagnosticOnStderrOnly(t *testing.T) {
	const argTok = "FAKE-ARG-TOKEN-777"
	plain := errors.New("list servers: hetzner API GET /servers: 500 server_error: secret-looking text")
	r := runCLI(t, cliEnv{Client: &errClient{fakeServerClient: fixtureClient(), list: plain}}, "server", "list", "--json")
	want, err := os.ReadFile(filepath.Join("..", "testdata", "json", "server_list_hetzner_error.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Stdout != string(want) {
		t.Errorf("stdout differs from the golden:\n%s", r.Stdout)
	}
	if !strings.Contains(r.Stderr, "500 server_error") {
		t.Errorf("stderr has no diagnostic: %q", r.Stderr)
	}

	leaks := []error{
		errors.New("list servers: hetzner API GET /servers: 500 x: " + fakeToken + " was refused"),
		errors.New("list servers: hetzner API GET /servers: 500 x: Authorization: Bearer abc.def"),
		errors.New("list servers: hetzner API GET /servers: 500 x: sent " + argTok + " in the request"),
	}
	for _, le := range leaks {
		r := runCLI(t, cliEnv{Client: &errClient{fakeServerClient: fixtureClient(), list: le}}, "server", "list", "--token", argTok, "--json")
		for _, bad := range []string{fakeToken, argTok, "abc.def", "Bearer", "Authorization"} {
			if strings.Contains(r.Stdout, bad) || strings.Contains(r.Stderr, bad) {
				t.Errorf("%q leaked (%v)\nstdout:\n%s\nstderr:\n%s", bad, le, r.Stdout, r.Stderr)
			}
		}
		if r.Stderr == "" {
			t.Errorf("empty stderr for %v", le)
		}
	}
}

// pflag takes the last --json value; so does wantsJSON.
func TestWantsJSONLastValueWins(t *testing.T) {
	cases := map[string]bool{"--json": true, "--json=false": false, "--json=false --json": true, "--json --json=false": false, "--json=1 --json=0": false, "-- --json": false, "": false}
	for in, want := range cases {
		if got := wantsJSON(strings.Fields(in)); got != want {
			t.Errorf("wantsJSON(%q) = %v, want %v", in, got, want)
		}
	}
}
