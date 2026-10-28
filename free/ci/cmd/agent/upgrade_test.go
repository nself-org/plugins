package main

// Purpose: `nself-ci-agent upgrade` (D-1385): registration, the usage exits, the --manifest-url name check, and that the version
//          run is wired to internal/exec's seam.
// Inputs:  the command table through run; a verifier that fails, so no test reaches a download or a swap of the test binary.
// Outputs: pass or fail.
// Constraints: nothing is fetched or installed here; the swap itself is covered by internal/agent/update's tests.

import (
	"errors"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/agent/verify"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
)

// failVerifier makes a run that gets past its flag checks stop before any network or file work.
func failVerifier(t *testing.T) {
	t.Helper()
	old := upgradeVerifier
	upgradeVerifier = func() (*verify.Verifier, error) { return nil, errors.New("verifier stop") }
	t.Cleanup(func() { upgradeVerifier = old })
}

func TestUpgradeIsRegisteredAndRunsTheCandidateThroughTheExecSeam(t *testing.T) {
	if _, ok := lookup("upgrade"); !ok {
		t.Fatal("upgrade is not in the agent's subcommand table")
	}
	if reflect.ValueOf(upgradeRunVersion).Pointer() != reflect.ValueOf(ciexec.RunAgentVersion).Pointer() {
		t.Fatal("upgrade does not start the new binary through internal/exec")
	}
}

func TestUpgradeUsageExits(t *testing.T) {
	failVerifier(t)
	for _, args := range [][]string{
		{"upgrade"},
		{"upgrade", "--version", ""},
		{"upgrade", "--version", "1.5"},
		{"upgrade", "--version", "v1.5.0"},
		{"upgrade", "--version", "1.5.0", "extra"},
		{"upgrade", "--version", "1.5.0", "--nope"},
		{"upgrade", "--version"},
		{"upgrade", "--version", "1.5.0", "--allow-downgrade=maybe"},
	} {
		if code, stdout, stderr := runCmd(t, args...); code != 2 || stdout != "" || !strings.Contains(stderr, "usage: nself-ci-agent upgrade") {
			t.Errorf("%v: code %d stdout %q stderr %q", args, code, stdout, stderr)
		}
	}
}

func TestUpgradeChecksTheManifestURLName(t *testing.T) {
	failVerifier(t)
	target := verify.Target(runtime.GOOS, runtime.GOARCH)
	good := "https://example.invalid/releases/v1.5.0/nself-ci-agent-" + target + ".manifest.json"
	for _, bad := range []string{
		"https://example.invalid/releases/v1.5.0/nself-ci-agent-other-arch.manifest.json",
		"https://example.invalid/releases/v1.5.0/nself-ci-agent-" + target + ".json",
		"https://example.invalid/releases/v1.5.0/",
		"https://example.invalid/releases/v1.5.0/nself-ci-agent-" + target + ".manifest.json/../x",
	} {
		code, _, stderr := runCmd(t, "upgrade", "--version", "1.5.0", "--manifest-url", bad)
		if code != 2 || !strings.Contains(stderr, "E651:") {
			t.Errorf("%q: code %d stderr %q, want exit 2 and E651", bad, code, stderr)
		}
	}
	// the right name passes the check and reaches the verifier, which stops the run with exit 1
	code, _, stderr := runCmd(t, "upgrade", "--version", "1.5.0", "--manifest-url", good)
	if code != 1 || !strings.Contains(stderr, "verifier stop") {
		t.Fatalf("good url: code %d stderr %q", code, stderr)
	}
}
