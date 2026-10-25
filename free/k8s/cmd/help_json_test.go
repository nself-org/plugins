// Purpose: prove `--help --json` prints one ok v1 envelope whose data.help is
// the exact text human --help prints, and that human --help is unchanged.
//
// Inputs: the built binary (cli_harness_test.go), args with --help or -h and
// --json in different positions, for the root and subcommands.
//
// Outputs: test results. No network, no helm, no cluster.
//
// Constraints: the human help is read from the same binary without --json, so
// the test pins "same text", not a particular wording.
package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestHelpJSONEnvelope(t *testing.T) {
	cases := []struct {
		name    string
		human   []string
		json    []string
		command string
	}{
		{"root", []string{"--help"}, []string{"--help", "--json"}, "k8s"},
		{"root short flag first", []string{"-h"}, []string{"--json", "-h"}, "k8s"},
		{"install", []string{"install", "--help"}, []string{"install", "--help", "--json"}, "k8s install"},
		{"status json first", []string{"status", "--help"}, []string{"--json", "status", "--help"}, "k8s status"},
		{"uninstall short", []string{"uninstall", "-h"}, []string{"uninstall", "--json", "-h"}, "k8s uninstall"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			human := runCLI(t, cliRun{}, tc.human...)
			if human.Exit != 0 || strings.TrimSpace(human.Stdout) == "" {
				t.Fatalf("human help: exit %d, stdout %q", human.Exit, human.Stdout)
			}
			res := runCLI(t, cliRun{}, tc.json...)
			if res.Exit != 0 {
				t.Fatalf("exit %d, stderr %q", res.Exit, res.Stderr)
			}
			d := mustEnvelope(t, res, tc.command)
			if d.Error != nil {
				t.Fatalf("want an ok envelope, got error %+v", d.Error)
			}
			help, _ := d.Data["help"].(string)
			if help == "" || help != human.Stdout {
				t.Fatalf("data.help differs from human help:\n--- envelope\n%q\n--- human\n%q", help, human.Stdout)
			}
			if len(d.Data) != 1 {
				t.Fatalf("data has extra keys: %v", d.Data)
			}
		})
	}
}

// TestHelpJSONFalseStaysHuman: --json=false with --help is plain help text.
func TestHelpJSONFalseStaysHuman(t *testing.T) {
	res := runCLI(t, cliRun{}, "install", "--help", "--json=false")
	if res.Exit != 0 || strings.HasPrefix(strings.TrimSpace(res.Stdout), "{") || !strings.Contains(res.Stdout, "Usage:") {
		t.Fatalf("want plain help, got exit %d %q", res.Exit, res.Stdout)
	}
}

// spyRunE wraps the RunE of every command under c and returns the call counter.
func spyRunE(t *testing.T, c *cobra.Command) *int {
	t.Helper()
	n := new(int)
	for _, sub := range c.Commands() {
		if orig := sub.RunE; orig != nil {
			sub := sub
			sub.RunE = func(cmd *cobra.Command, args []string) error { *n++; return orig(cmd, args) }
			t.Cleanup(func() { sub.RunE = orig })
		}
	}
	return n
}

// TestHelpFlagValueIsNotHelp: "-h" or "--help" as the VALUE of another flag is not a
// help request. helpJSON must answer handled=false, print nothing and never run a
// RunE (it used to Execute the command, which ran it and then printed an ok help).
func TestHelpFlagValueIsNotHelp(t *testing.T) {
	cases := [][]string{
		{"install", "--domain", "-h", "--json"},
		{"--json", "install", "--domain", "-h"},
		{"install", "--domain=-h", "--json"},
		{"upgrade", "--release", "-h", "--json"},
		{"uninstall", "--release", "-h", "--yes", "--json"},
		{"status", "--release", "--help", "--json"},
	}
	n := spyRunE(t, rootCmd)
	for _, args := range cases {
		var out, errb bytes.Buffer
		handled, _ := helpJSON(rootCmd, args, &out, &errb)
		if handled || out.Len() != 0 || *n != 0 {
			t.Errorf("%v: handled=%v stdout=%q RunE calls=%d, want unhandled, silent, 0 calls", args, handled, out.String(), *n)
		}
		*n = 0
	}
	// The built binary takes the normal route and never prints an ok help envelope.
	res := runCLI(t, cliRun{Helm: "ok"}, "uninstall", "--release", "-h", "--yes", "--json")
	d := mustEnvelope(t, res, "k8s uninstall")
	if _, has := d.Data["help"]; has || d.Error == nil || res.Exit == 0 {
		t.Fatalf("want an error envelope from the normal path, got exit %d:\n%s", res.Exit, res.Stdout)
	}
	// Control: a real help request still answers, still without a RunE.
	var out bytes.Buffer
	if handled, code := helpJSON(rootCmd, []string{"install", "--domain", "x", "-h", "--json"}, &out, &bytes.Buffer{}); !handled || code != 0 || *n != 0 {
		t.Fatalf("control: handled=%v code=%d RunE calls=%d", handled, code, *n)
	}
}

// TestHelpJSONOnlyStripsValidJSONForms: the probe drops --json spellings pflag accepts for a
// bool and keeps anything else, so `--json=maybe` makes the flag parse fail and the normal path
// (which refuses it) runs, instead of the probe answering help for an invalid request.
func TestHelpJSONOnlyStripsValidJSONForms(t *testing.T) {
	for _, tok := range []string{"--json", "--json=true", "--json=1", "--json=T"} {
		var out bytes.Buffer
		if handled, code := helpJSON(rootCmd, []string{"install", "--help", tok}, &out, &bytes.Buffer{}); !handled || code != 0 {
			t.Errorf("install --help %s: handled=%v code=%d, want handled", tok, handled, code)
		}
	}
	for _, tok := range []string{"--json=maybe", "--json=", "--json=yes"} {
		var out bytes.Buffer
		if handled, _ := helpJSON(rootCmd, []string{"install", "--help", "--json", tok}, &out, &bytes.Buffer{}); handled || out.Len() != 0 {
			t.Errorf("install --help --json %s: handled=%v stdout=%q, want unhandled and silent", tok, handled, out.String())
		}
	}
}
