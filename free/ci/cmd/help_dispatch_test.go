package main

import (
	"strings"
	"testing"
)

// stubAllHandlers swaps every registered handler of both tables for one that records its key and
// returns 99, and restores the tables when the test ends. A help request that reaches a handler (or the
// gate or the bare run behind it) is then visible as a recorded key or a non-zero exit.
func stubAllHandlers(t *testing.T) *[]string {
	t.Helper()
	var ran []string
	old14, old15 := subcommands, v15Subcommands
	subcommands, v15Subcommands = map[string]handler{}, map[string]subcommandEntry{}
	for k := range old14 {
		key := "v14 " + k
		subcommands[k] = func([]string) int { ran = append(ran, key); return 99 }
	}
	for k, e := range old15 {
		key, entry := "v15 "+k, e
		entry.Handler = func([]string) int { ran = append(ran, key); return 99 }
		v15Subcommands[k] = entry
	}
	t.Cleanup(func() { subcommands, v15Subcommands = old14, old15 })
	return &ran
}

// allKeys is every key of both tables, hidden entries included.
func allKeys() []string {
	seen := map[string]bool{}
	var out []string
	for k := range subcommands {
		seen[k] = true
	}
	for k := range v15Subcommands {
		seen[k] = true
	}
	for k := range seen {
		out = append(out, k)
	}
	return out
}

// Asking any registered subcommand for help prints help, exits 0 and runs nothing: not the handler, not
// the nodes --coordinator client (which would dial the URL), not the bare run or the gate. The table is
// built from the registries, so a new subcommand is covered the day it is registered.
func TestEverySubcommandHelpRunsNothing(t *testing.T) {
	keys := allKeys()
	if len(keys) < 40 {
		t.Fatalf("only %d registered keys; the registries did not load", len(keys))
	}
	ran := stubAllHandlers(t)
	t.Setenv("NSELF_CI_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	for _, key := range keys {
		for _, extra := range [][]string{
			{"--help"}, {"-h"}, {"-help"},
			{"--coordinator", "http://127.0.0.1:1", "--help"}, // must not dial
		} {
			args := append(strings.Fields(key), extra...)
			// a core-help key keeps core's rule: a flag that does not parse before the help token is an error
			_, core := coreHelp[key]
			parseErr := core && (extra[0] == "-help" || extra[0] == "--coordinator")
			for name, dispatch := range map[string]func([]string) int{"run": run, "runV15": runV15} {
				var code int
				out, errOut := captureStd(t, func() { code = dispatch(args) })
				if parseErr {
					if code != 1 || len(*ran) != 0 || !strings.Contains(errOut, "Error: ") {
						t.Errorf("%s %q: exit %d, handlers run %v, stderr %q; want core's parse error, nothing run", name, args, code, *ran, errOut)
						*ran = nil
					}
					continue
				}
				if code != 0 || len(*ran) != 0 || strings.TrimSpace(out) == "" {
					t.Errorf("%s %q: exit %d, handlers run %v, stdout %q, stderr %q; want exit 0, nothing run, help on stdout",
						name, args, code, *ran, out, errOut)
					*ran = nil
				}
			}
		}
	}
}

// A help request after the "--" terminator belongs to the command being run, not to nself ci.
func TestHelpAfterTerminatorIsNotHelp(t *testing.T) {
	if wantsHelp([]string{"--", "--help"}) || wantsHelp([]string{"-h", "--", "x"}) == false {
		t.Fatal("wantsHelp must stop at the terminator and see a flag before it")
	}
}

// A group word with no command of its own lists its children and runs nothing.
func TestGroupHelpListsChildren(t *testing.T) {
	ran := stubAllHandlers(t)
	t.Chdir(t.TempDir())
	for group, child := range map[string]string{"config": "config show", "nodes pools": "nodes pools list", "policy": "policy show"} {
		var code int
		out, _ := captureStd(t, func() { code = runV15(append(strings.Fields(group), "--help")) })
		if code != 0 || len(*ran) != 0 || !strings.Contains(out, child) {
			t.Errorf("%s --help: exit %d, ran %v, stdout %q; want exit 0, nothing run, %q listed", group, code, *ran, out, child)
		}
	}
}
