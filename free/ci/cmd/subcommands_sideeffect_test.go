package main

// Purpose: test the side-effect class on the V15 subcommand table (D-1085):
// the class is a valid, closed set and the read-only commands carry it, so
// refgen and the commands.yaml generator can read it from the table.

import (
	"strings"
	"testing"
)

// TestSideEffectClassIsValidated proves tableProblems rejects an unknown class
// and accepts the three defined ones and an unclassified entry.
func TestSideEffectClassIsValidated(t *testing.T) {
	ok := func([]string) int { return 0 }
	entry := func(c SideEffect) subcommandEntry {
		return subcommandEntry{Handler: ok, V15Only: true, Flags: runsFlags, SideEffect: c}
	}
	good := map[string]subcommandEntry{
		"a": entry(SideEffectRead), "b": entry(SideEffectRemote),
		"c": entry(SideEffectDestructive), "d": entry(""),
	}
	if p := tableProblems(good); len(p) != 0 {
		t.Fatalf("valid classes flagged: %v", p)
	}
	p := tableProblems(map[string]subcommandEntry{"x": entry("explode")})
	if len(p) != 1 || !strings.Contains(p[0], "unknown side effect") {
		t.Fatalf("unknown class not flagged: %v", p)
	}
}

// TestReadOnlyCommandsAreClassifiedRead proves the commands that only read
// state carry the read class in the table.
func TestReadOnlyCommandsAreClassifiedRead(t *testing.T) {
	for _, k := range []string{"config show", "doctor", "explain", "plan", "logs", "runs follow", "runs wait", "evidence verify"} {
		e, found := v15Subcommands[k]
		if !found {
			t.Errorf("%q not registered", k)
			continue
		}
		if e.SideEffect != SideEffectRead {
			t.Errorf("%q side effect = %q, want %q", k, e.SideEffect, SideEffectRead)
		}
	}
}

// TestNodeSecretsAreClassifiedRemote proves the commands that change grants on
// the coordinator carry the remote class.
func TestNodeSecretsAreClassifiedRemote(t *testing.T) {
	for _, k := range []string{"nodes secrets grant", "nodes secrets revoke"} {
		e, found := v15Subcommands[k]
		if !found {
			t.Errorf("%q not registered", k)
			continue
		}
		if e.SideEffect != SideEffectRemote {
			t.Errorf("%q side effect = %q, want %q", k, e.SideEffect, SideEffectRemote)
		}
	}
}
