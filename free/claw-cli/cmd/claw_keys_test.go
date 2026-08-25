// G1-T10: claw_keys --bootstrap headless-mode tests.
//
// Tests cover the validation gate (missing flags, bad tier, bad email all exit
// with bootstrapExitCode) and the happy-path POST against a fake server.
//
// Moved from cli/cmd/commands/claw_keys_test.go under CLI-R11:
// internal/errs.ExitCoder -> local exitCoder (exit.go); captureStderr, which
// core's deprecation_wiring_test.go defined for the whole cmd/commands
// package, is copied in below since this plugin has no equivalent shared
// test file.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// captureStderr runs fn with os.Stderr replaced by a pipe and returns what was
// written.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		done <- buf.String()
	}()

	fn()

	os.Stderr = orig
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}

func TestBootstrap_ValidTiers(t *testing.T) {
	wantOK := []string{"owner", "plus", "claw", "chat", "media", "family", "pro", "enterprise"}
	for _, tier := range wantOK {
		if !validBootstrapTiers[tier] {
			t.Errorf("expected tier %q to be valid", tier)
		}
	}
	if validBootstrapTiers["bogus"] {
		t.Error("unexpected tier 'bogus' marked valid")
	}
	if validBootstrapTiers[""] {
		t.Error("empty tier should not be valid")
	}
}

// setBootstrapFlags sets the package-level bootstrap flag vars and restores
// them afterwards, so one failing case cannot leak into the next.
func setBootstrapFlags(t *testing.T, owner, tier, machineID string) {
	t.Helper()
	prevBootstrap, prevName := clawKeysCreateBootstrap, clawKeysCreateName
	prevOwner, prevTier, prevMachine := clawKeysCreateOwner, clawKeysCreateTier, clawKeysCreateMachineID
	t.Cleanup(func() {
		clawKeysCreateBootstrap, clawKeysCreateName = prevBootstrap, prevName
		clawKeysCreateOwner, clawKeysCreateTier, clawKeysCreateMachineID = prevOwner, prevTier, prevMachine
	})
	clawKeysCreateBootstrap = true
	clawKeysCreateName = "ci-key"
	clawKeysCreateOwner = owner
	clawKeysCreateTier = tier
	clawKeysCreateMachineID = machineID
}

// assertBootstrapRejects runs the bootstrap path and asserts it refuses with
// bootstrapExitCode and prints the expected fragments on stderr.
func assertBootstrapRejects(t *testing.T, wantFragments ...string) {
	t.Helper()

	stderr := captureStderr(t, func() {
		err := runClawKeysCreateBootstrap(nil)
		if err == nil {
			t.Error("expected a validation error, got nil")
			return
		}
		var coder exitCoder
		if !errors.As(err, &coder) {
			t.Errorf("error does not carry an exit code: %v", err)
			return
		}
		if coder.ExitCode() != bootstrapExitCode {
			t.Errorf("expected exit code %d, got %d", bootstrapExitCode, coder.ExitCode())
		}
	})

	for _, want := range wantFragments {
		if !strings.Contains(stderr, want) {
			t.Errorf("expected stderr to contain %q, got: %s", want, stderr)
		}
	}
}

func TestBootstrapValidation_MissingFlags(t *testing.T) {
	setBootstrapFlags(t, "", "", "")
	assertBootstrapRejects(t, "--owner-email", "--tier", "--machine-id")
}

func TestBootstrapValidation_InvalidTier(t *testing.T) {
	setBootstrapFlags(t, "ci@example.com", "bogus", "m1")
	assertBootstrapRejects(t, "invalid --tier")
}

func TestBootstrapValidation_InvalidEmail(t *testing.T) {
	setBootstrapFlags(t, "not-an-email", "owner", "m1")
	assertBootstrapRejects(t, "valid email")
}

// TestBootstrap_HappyPath spins up a fake claw server, points the CLI's
// server-URL config at it, and confirms that the bootstrap path emits the
// raw key on stdout exactly as expected. We exercise the function directly
// (no subprocess) since the validation passes.
func TestBootstrap_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/claw/v1/api-keys/bootstrap" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["owner_email"] != "ci@example.com" || body["tier"] != "owner" || body["machine_id"] != "host-1" {
			t.Errorf("unexpected request body: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"key": "nself_pro_TESTKEY1234567890abcdef",
			"id":  "key-uuid-1",
		})
	}))
	defer srv.Close()

	t.Setenv("NSELF_CLAW_SERVER", srv.URL)

	clawKeysCreateBootstrap = true
	clawKeysCreateName = "ci-key"
	clawKeysCreateOwner = "ci@example.com"
	clawKeysCreateTier = "owner"
	clawKeysCreateMachineID = "host-1"
	defer func() {
		clawKeysCreateBootstrap = false
		clawKeysCreateName = ""
		clawKeysCreateOwner = ""
		clawKeysCreateTier = ""
		clawKeysCreateMachineID = ""
	}()

	stdout := captureLocalStdout(t, func() {
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())
		if err := runClawKeysCreateBootstrap(cmd); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if strings.TrimSpace(stdout) != "nself_pro_TESTKEY1234567890abcdef" {
		t.Fatalf("expected raw key on stdout, got: %q", stdout)
	}
}

// captureLocalStdout swaps os.Stdout for a pipe, runs fn, and returns
// everything it wrote.
func captureLocalStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	_ = w.Close()
	os.Stdout = orig
	return <-done
}
