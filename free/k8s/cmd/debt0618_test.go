package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/nself-k8s/internal/k8s"
	"github.com/spf13/cobra"
)

func TestD0618HelmDiagnostics(t *testing.T) {
	for _, js := range []bool{false, true} {
		t.Run(fmt.Sprint(js), func(t *testing.T) {
			he := &k8s.HelmError{Verb: "upgrade", Err: errors.New("failed"),
				Hint:   "If helm applied the upgrade, it rolled back; run helm status rel",
				Stderr: "\x1b[31mconnection refused https://user:URLSECRET@cluster.test/PATHSECRET?token=QUERYSECRET\x1b[0m password=PASSSECRET"}
			var stdout strings.Builder
			cmd := &cobra.Command{}
			cmd.SetOut(&stdout)
			err := (&invocation{cmd: cmd, sub: "upgrade", json: js}).fail(fmt.Errorf("wrapped: %w", he))
			var text string
			if js {
				var doc struct {
					Error output.ErrorDetail `json:"error"`
				}
				if e := json.Unmarshal([]byte(stdout.String()), &doc); e != nil {
					t.Fatal(e)
				}
				text = doc.Error.Remediation + " " + doc.Error.Cause
				if doc.Error.Code != CodeHelmFailed {
					t.Fatalf("code: %s", doc.Error.Code)
				}
			} else {
				text = err.Error()
			}
			for _, want := range []string{"rolled back", "helm status rel", "connection refused", "cluster.test"} {
				if !strings.Contains(text, want) {
					t.Errorf("missing %q in %q", want, text)
				}
			}
			for _, secret := range []string{"URLSECRET", "PATHSECRET", "QUERYSECRET", "PASSSECRET", "\x1b"} {
				if strings.Contains(text, secret) {
					t.Errorf("unsafe diagnostic: %q", text)
				}
			}
		})
	}
}

func TestD0618BoundedDiagnostic(t *testing.T) {
	d := classify(&k8s.HelmError{Verb: "status", Stderr: strings.Repeat("界", 600) + " END"})
	if len(d.Cause) > 515 || !utf8.ValidString(d.Cause) || !strings.HasSuffix(d.Cause, " END") {
		t.Fatalf("diagnostic must be bounded UTF-8 tail: %q (%d bytes)", d.Cause, len(d.Cause))
	}
}

func TestD0618ContextFlag(t *testing.T) {
	for _, original := range []*cobra.Command{installCmd, upgradeCmd} {
		t.Run(original.Name(), func(t *testing.T) {
			if original.Flags().Lookup("context") == nil {
				t.Fatal("--context not registered")
			}
			cmd := &cobra.Command{}
			addInstallFlags(cmd)
			if err := cmd.Flags().Parse([]string{"--context", "explicit-ctx"}); err != nil {
				t.Fatal(err)
			}
			if got := installOptions(cmd).KubeContext; got != "explicit-ctx" {
				t.Fatalf("context = %q", got)
			}
		})
	}
}

func TestD0618StatusDiagnosticCLI(t *testing.T) {
	for _, js := range []bool{false, true} {
		args := []string{"status"}
		if js {
			args = append(args, "--json")
		}
		res := runCLI(t, cliRun{Helm: "fail"}, args...)
		if res.Exit == 0 || !strings.Contains(res.Stdout+res.Stderr, "stub helm: boom") {
			t.Fatalf("status lost diagnostics (json=%t): %#v", js, res)
		}
	}
}

func TestD0618SanitizeCredentials(t *testing.T) {
	t.Setenv(k8s.LicenseEnv, "configured-license")
	he := &k8s.HelmError{Verb: "status", Stderr: "timeout token='private token' Authorization: Bearer PRIVATE password=PRIVATE configured-license\x1b]0;terminal-title\x07"}
	got := classify(he).Cause
	if !strings.Contains(got, "timeout") {
		t.Fatalf("lost diagnosis: %q", got)
	}
	for _, secret := range []string{"private token", "PRIVATE", "configured-license", "terminal-title", "\x1b", "\x07"} {
		if strings.Contains(got, secret) {
			t.Errorf("secret/control %q in %q", secret, got)
		}
	}
}
