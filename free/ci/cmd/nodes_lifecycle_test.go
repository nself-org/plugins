package main

// Purpose: the nine node lifecycle keys are registered V15-only with a side-effect class, and the v1.4
// dispatcher does not see them (parity with the frozen argv).

import (
	"strings"
	"testing"
)

func TestNodesLifecycleKeysAreV15Only(t *testing.T) {
	want := map[string]SideEffect{"add": "remote", "list": "read", "show": "read", "test": "remote", "drain": "remote",
		"enable": "remote", "revoke": "destructive", "remove": "destructive", "rotate": "remote"}
	if len(nodesLifecycleSubs) != len(want) {
		t.Fatalf("%d subcommands, want %d", len(nodesLifecycleSubs), len(want))
	}
	for _, s := range nodesLifecycleSubs {
		key := "nodes " + s.name
		e, ok := v15Subcommands[key]
		if !ok || !e.V15Only || e.Handler == nil || e.Flags == nil {
			t.Errorf("%s: not a complete V15 entry: %+v", key, e)
		}
		if e.SideEffect != want[s.name] {
			t.Errorf("%s: table side effect %q, want %q", key, e.SideEffect, want[s.name])
		}
		if want[s.name] != s.class {
			t.Errorf("%s: side-effect class %q, want %q", key, s.class, want[s.name])
		}
		if _, v14 := subcommands[key]; v14 {
			t.Errorf("%s leaked into the v1.4 table", key)
		}
		if fs := e.Flags(); fs.Lookup("json") == nil {
			t.Errorf("%s: no --json", key)
		}
	}
}

func TestNodesLifecycleRefusesBadArgvThroughRun(t *testing.T) {
	t.Setenv("NSELF_CI_HOME", t.TempDir())
	code, _, errs := capture(t, func() int { return runV15([]string{"nodes", "revoke"}) })
	if code != 1 || !strings.Contains(errs, "usage: nself ci nodes revoke") {
		t.Fatalf("code=%d err=%q", code, errs)
	}
}
