// Purpose: prove `--help --json` prints one ok v1 envelope whose data.help is
// the exact text human --help prints, and that human --help is unchanged.
//
// Inputs: the in-process harness (cli_harness_test.go), args with --help or -h
// and --json in different positions, for the root and subcommands.
//
// Outputs: test results. No network, no cloud credentials, no spend.
//
// Constraints: the human help is read from the same run without --json, so the
// test pins "same text", not a particular wording.
package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/output"
)

func TestHelpJSONEnvelope(t *testing.T) {
	cases := []struct {
		name    string
		human   []string // the human-mode args
		json    []string // the --json args
		command string
	}{
		{"root", []string{"--help"}, []string{"--help", "--json"}, "infra"},
		{"root short flag first", []string{"-h"}, []string{"--json", "-h"}, "infra"},
		{"plan", []string{"plan", "--help"}, []string{"plan", "--help", "--json"}, "infra plan"},
		{"apply json first", []string{"apply", "--help"}, []string{"--json", "apply", "--help"}, "infra apply"},
		{"server list", []string{"server", "list", "--help"}, []string{"server", "list", "--json", "-h"}, "infra server list"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			human := runCLI(t, cliEnv{}, tc.human...)
			if human.Exit != 0 || strings.TrimSpace(human.Stdout) == "" {
				t.Fatalf("human help: exit %d, stdout %q", human.Exit, human.Stdout)
			}
			res := runCLI(t, cliEnv{}, tc.json...)
			if res.Exit != 0 {
				t.Fatalf("exit %d, stderr %q", res.Exit, res.Stderr)
			}
			if err := output.CheckEnvelope([]byte(res.Stdout)); err != nil {
				t.Fatalf("not a v1 envelope: %v\n%s", err, res.Stdout)
			}
			var doc envDoc
			if err := json.Unmarshal([]byte(res.Stdout), &doc); err != nil {
				t.Fatal(err)
			}
			if doc.Error != nil || doc.Command != tc.command {
				t.Fatalf("command %q error %v, want ok %q", doc.Command, doc.Error, tc.command)
			}
			help, _ := doc.Data["help"].(string)
			if help == "" || help != human.Stdout {
				t.Fatalf("data.help differs from human help:\n--- envelope\n%q\n--- human\n%q", help, human.Stdout)
			}
			if len(doc.Data) != 1 {
				t.Fatalf("data has extra keys: %v", doc.Data)
			}
		})
	}
}

// TestHelpJSONFalseStaysHuman: --json=false with --help is plain help text.
func TestHelpJSONFalseStaysHuman(t *testing.T) {
	res := runCLI(t, cliEnv{}, "plan", "--help", "--json=false")
	if res.Exit != 0 || strings.HasPrefix(strings.TrimSpace(res.Stdout), "{") || !strings.Contains(res.Stdout, "Usage:") {
		t.Fatalf("want plain help, got exit %d %q", res.Exit, res.Stdout)
	}
}

// TestHumanHelpBytes pins the human help on the points the issue touched:
// the nself prefix, the hetzner-only provider flag and the OpenTofu text.
func TestHumanHelpBytes(t *testing.T) {
	res := runCLI(t, cliEnv{}, "apply", "--help")
	for _, want := range []string{"nself infra apply [flags]", "Cloud provider (hetzner)", "tofu apply"} {
		if !strings.Contains(res.Stdout, want) {
			t.Errorf("apply --help lacks %q:\n%s", want, res.Stdout)
		}
	}
	root := runCLI(t, cliEnv{}, "--help")
	for _, bad := range []string{"azure, hetzner", "do, linode)"} {
		if strings.Contains(root.Stdout+res.Stdout, bad) {
			t.Errorf("help still lists unshipped providers (%q)", bad)
		}
	}
	for _, want := range []string{"Hetzner is the only shipped provider", "--i-accept-cloud-spend", "tofu is used first"} {
		if !strings.Contains(root.Stdout, want) {
			t.Errorf("root help lacks %q", want)
		}
	}
}
