package main

// Purpose: the agent's StaleChecker and incarnation ids (D-1002).

import (
	"context"
	"os"
	"strconv"
	"testing"
)

func TestAgentStaleChecker(t *testing.T) {
	skipUnixOnly(t)
	self := agentIncarnation("coord-a")
	other := agentIncarnation("coord-b")
	if self == other {
		t.Fatal("two coordinators share an incarnation id")
	}
	ctx := context.Background()
	if stale, err := (agentStale{}).Stale(ctx, self); err != nil || stale {
		t.Fatalf("a live agent is not stale: %v %v", stale, err)
	}
	for _, id := range []string{"local", "ag-x", "ag-0000000000-999999999-1", "ag-0000000000-" + strconv.Itoa(os.Getpid()) + "-wrongstart"} {
		stale, err := (agentStale{}).Stale(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		// unparsable ids are never swept; a dead pid or a reused pid (other start time) is
		want := id != "local" && id != "ag-x"
		if stale != want {
			t.Fatalf("Stale(%q) = %v, want %v", id, stale, want)
		}
	}
}
