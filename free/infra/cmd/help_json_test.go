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
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/spf13/cobra"
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

// TestHelpJSONInvalidValueNotStripped (D-0615): only a --json spelling pflag accepts
// as a bool is dropped from the probe. An invalid one (--json=maybe) stays, so the
// probe cannot swallow a value the normal path would refuse: no help envelope.
func TestHelpJSONInvalidValueNotStripped(t *testing.T) {
	res := runCLI(t, cliEnv{}, "plan", "--help", "--json", "--json=maybe")
	if res.Exit == 0 || strings.Contains(res.Stdout, `"help"`) {
		t.Fatalf("want the invalid --json value refused, got exit %d stdout %q", res.Exit, res.Stdout)
	}
	for _, ok := range []string{"--json=true", "--json=1", "--json=T"} {
		res := runCLI(t, cliEnv{}, "plan", "--help", ok)
		if res.Exit != 0 || !strings.Contains(res.Stdout, `"help"`) {
			t.Fatalf("%s: want a help envelope, got exit %d stdout %q", ok, res.Exit, res.Stdout)
		}
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

// spyRunE wraps the RunE of every command under c and returns the call counter.
func spyRunE(t *testing.T, c *cobra.Command) *int {
	t.Helper()
	n := new(int)
	var wrap func(*cobra.Command)
	wrap = func(c *cobra.Command) {
		if orig := c.RunE; orig != nil {
			c.RunE = func(cmd *cobra.Command, args []string) error { *n++; return orig(cmd, args) }
			t.Cleanup(func() { c.RunE = orig })
		}
		for _, sub := range c.Commands() {
			wrap(sub)
		}
	}
	wrap(c)
	return n
}

// TestHelpFlagValueIsNotHelp: "-h" or "--help" as the VALUE of another flag is not a
// help request. helpJSON must answer handled=false, print nothing and never run a
// RunE (it used to Execute the command, which ran it and then printed an ok help).
func TestHelpFlagValueIsNotHelp(t *testing.T) {
	rootOnce.Do(configureRoot)
	cases := [][]string{
		{"apply", "--provider", "hetzner", "--domain", "-h", "--i-accept-cloud-spend", "--auto-approve", "--json"},
		{"--json", "apply", "--provider", "hetzner", "--domain", "-h"},
		{"apply", "--domain=-h", "--provider", "hetzner", "--json"},
		{"plan", "--domain", "--help", "--json"},
		{"server", "provision", "--name", "-h", "--type", "cx22", "--location", "fsn1", "--image", "x", "--json"},
		{"server", "provision", "--label", "-h", "--name", "a", "--json"},
		{"server", "destroy", "--id", "1", "--token", "-h", "--force-no-backup", "--json"},
		{"server", "resize", "--id", "1", "--type", "-h", "--json"},
	}
	n := spyRunE(t, infraCmd)
	for _, args := range cases {
		var out, errb bytes.Buffer
		handled, _ := helpJSON(infraCmd, args, &out, &errb)
		if handled || out.Len() != 0 || *n != 0 {
			t.Errorf("%v: handled=%v stdout=%q RunE calls=%d, want unhandled, silent, 0 calls", args, handled, out.String(), *n)
		}
		*n = 0
	}
	// The full path takes the normal route: the spend gate refuses (no terminal).
	res := runCLI(t, cliEnv{Tofu: "ok"}, cases[0]...)
	var doc envDoc
	if err := json.Unmarshal([]byte(res.Stdout), &doc); err != nil {
		t.Fatalf("stdout is not an envelope: %v\n%s", err, res.Stdout)
	}
	if _, has := doc.Data["help"]; has || doc.Error == nil || doc.Error.Code != "E403" || res.Exit != 4 || res.TofuLog != "" {
		t.Fatalf("want the E403 spend refusal, no tofu call; got exit %d log %q:\n%s", res.Exit, res.TofuLog, res.Stdout)
	}
	*n = 0 // the full run above legitimately ran apply once
	// Control: a real help request still answers, still without a RunE.
	var out bytes.Buffer
	if handled, code := helpJSON(infraCmd, []string{"apply", "--domain", "x", "-h", "--json"}, &out, &bytes.Buffer{}); !handled || code != 0 || *n != 0 {
		t.Fatalf("control: handled=%v code=%d RunE calls=%d", handled, code, *n)
	}
}

// TestHelpJSONLeavesFlagsClean: the parse inside helpJSON must not leak flag
// values into the normal run that follows (slice flags would append twice).
func TestHelpJSONLeavesFlagsClean(t *testing.T) {
	rootOnce.Do(configureRoot)
	var out bytes.Buffer
	args := []string{"server", "provision", "--label", "a=b", "--name", "n", "--help", "--json"}
	if handled, _ := helpJSON(infraCmd, args, &out, &bytes.Buffer{}); !handled {
		t.Fatal("want handled")
	}
	f := serverProvisionCmd.Flags()
	if l, _ := f.GetStringArray("label"); len(l) != 0 {
		t.Errorf("label leaked: %v", l)
	}
	if v, _ := f.GetString("name"); v != "" || f.Lookup("name").Changed {
		t.Errorf("name leaked: %q changed=%v", v, f.Lookup("name").Changed)
	}
}
