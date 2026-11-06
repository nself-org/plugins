package main

// Tests for "nself claw unlock" (prints the token) and "nself claw setup"
// (posts it to /claw/auth/setup). They run against an httptest server that
// follows the bundles#343 contract; no live claw server is needed.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

var testUnlockToken = strings.Join([]string{"unlock", "token", "for", "tests"}, "-")

func newTestCmd(stdin string) (*cobra.Command, *bytes.Buffer) {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetIn(strings.NewReader(stdin))
	return cmd, out
}

func TestUnlockPrintsToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/claw/auth/unlock" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"token":      testUnlockToken,
			"expires_at": "2026-11-06T12:00:00Z",
		})
	}))
	defer srv.Close()
	t.Setenv("PLUGIN_CLAW_INTERNAL_URL", srv.URL)

	clawUnlockMinutes = 10
	cmd, out := newTestCmd("")
	if err := runClawUnlock(cmd, nil); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	if !strings.Contains(out.String(), testUnlockToken) {
		t.Fatalf("output lacks the token the server returned:\n%s", out.String())
	}
}

func TestSetupSendsUnlockToken(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/claw/auth/unlock":
			_ = json.NewEncoder(w).Encode(map[string]string{"token": testUnlockToken, "expires_at": "2026-11-06T12:00:00Z"})
		case "/claw/auth/setup":
			if r.Method != http.MethodPost {
				http.Error(w, "method", http.StatusMethodNotAllowed)
				return
			}
			_ = json.NewDecoder(r.Body).Decode(&got)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"user_id": "u1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("PLUGIN_CLAW_INTERNAL_URL", srv.URL)

	// Unlock, then feed the printed token to setup.
	clawUnlockMinutes = 10
	ucmd, uout := newTestCmd("")
	if err := runClawUnlock(ucmd, nil); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	var token string
	for _, f := range strings.Fields(uout.String()) {
		if f == testUnlockToken {
			token = f
		}
	}
	if token == "" {
		t.Fatalf("could not find token in unlock output:\n%s", uout.String())
	}

	clawSetupToken, clawSetupName, clawSetupPasswordStdin = token, "Ali", true
	scmd, sout := newTestCmd("correct horse battery\n")
	if err := runClawSetup(scmd, nil); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if got["unlock_token"] != testUnlockToken {
		t.Errorf("unlock_token = %q, want %q", got["unlock_token"], testUnlockToken)
	}
	if got["display_name"] != "Ali" {
		t.Errorf("display_name = %q", got["display_name"])
	}
	if got["password"] != "correct horse battery" {
		t.Errorf("password = %q (trailing newline must be trimmed)", got["password"])
	}
	if strings.Contains(sout.String(), "correct horse") {
		t.Errorf("setup output printed the password: %s", sout.String())
	}
}

func TestSetupRefusesPasswordFlag(t *testing.T) {
	if clawSetupCmd.Flags().Lookup("password") != nil {
		t.Fatal("setup must not define a --password flag")
	}
	if err := clawSetupCmd.ParseFlags([]string{"--password", "secret"}); err == nil {
		t.Error("--password must be refused as an unknown flag")
	}
	if err := clawSetupCmd.Args(clawSetupCmd, []string{"secret"}); err == nil {
		t.Error("a password argument must be refused")
	}

	// Without --password-stdin, nothing is sent and the run fails.
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer srv.Close()
	t.Setenv("PLUGIN_CLAW_INTERNAL_URL", srv.URL)
	clawSetupToken, clawSetupName, clawSetupPasswordStdin = testUnlockToken, "Ali", false
	cmd, _ := newTestCmd("secret\n")
	if err := runClawSetup(cmd, nil); err == nil {
		t.Error("setup without --password-stdin must fail")
	}
	if called {
		t.Error("setup contacted the server without --password-stdin")
	}
}
