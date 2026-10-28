package exec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeEvents sets the lifecycle events the fake runtime answers an `events` query with.
func fakeEvents(t *testing.T, statuses ...string) {
	t.Helper()
	lines := make([]string, len(statuses))
	for i, s := range statuses {
		lines[i] = `{"Type":"container","Action":"` + s + `"}`
	}
	body := strings.Join(lines, "\n")
	if err := os.WriteFile(filepath.Join(fakeRuntimeDir, "events"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestExit137IsVerifiedAgainstTheRuntime(t *testing.T) {
	for name, tc := range map[string]struct {
		events []string
		reason string
	}{
		"oom":           {[]string{"create", "start", "oom", "die"}, "resource_limit"},
		"operatorkill":  {[]string{"create", "start", "kill", "die"}, ""},
		"oomthenkill":   {[]string{"create", "start", "oom", "kill", "die"}, "resource_limit"},
		"nokillrecord":  {[]string{"create", "start", "die"}, "resource_limit"}, // the oom event can be lost under --rm
		"runtimesilent": {nil, "resource_limit"},                                // no answer proves nothing: the exit code is read as before
	} {
		t.Run(name, func(t *testing.T) {
			fakeRuntime(t, "", "137")
			fakeEvents(t, tc.events...)
			res := (&Executor{}).Run(context.Background(), plannedSpec(t, "plan-137-"+name, hardPlanner{}))
			if res.Reason != tc.reason {
				t.Fatalf("reason %q want %q (%+v)", res.Reason, tc.reason, res)
			}
		})
	}
}

func TestExit125IsVerifiedAgainstTheRuntime(t *testing.T) {
	t.Run("jobs own exit", func(t *testing.T) {
		fakeRuntime(t, "", "125")
		fakeEvents(t, "create", "start", "die")
		res := (&Executor{}).Run(context.Background(), plannedSpec(t, "plan-125-job", hardPlanner{}))
		if res.Err != nil || res.ExitCode != 125 || res.Attempt.FailureClass != "code" {
			t.Fatalf("a job that ran and exited 125 is a code failure, not a runtime one: %+v", res)
		}
	})
	t.Run("runtime failed", func(t *testing.T) {
		fakeRuntime(t, "", "125")
		fakeEvents(t, "create", "destroy") // created, never started
		res := (&Executor{}).Run(context.Background(), plannedSpec(t, "plan-125-rt", hardPlanner{}))
		var e *Error
		if !errors.As(res.Err, &e) || e.Code != "E613" {
			t.Fatalf("a container that never started is an E613 runtime failure: %+v", res)
		}
	})
}

func TestResourceReasonWithFacts(t *testing.T) {
	ctx := context.Background()
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	for name, tc := range map[string]struct {
		ctx    context.Context
		code   int
		tail   string
		facts  containerFacts
		reason string
	}{
		"137 oom":               {ctx, 137, "", containerFacts{known: true, started: true, oomed: true}, "resource_limit"},
		"137 unknown":           {ctx, 137, "", containerFacts{}, "resource_limit"},
		"137 cancelled":         {cancelled, 137, "", containerFacts{known: true, oomed: true}, ""},
		"fork refused, no oom":  {ctx, 2, "sh: can't fork", containerFacts{known: true, started: true}, "resource_limit"},
		"137 no oom, fork tail": {ctx, 137, "Cannot fork", containerFacts{known: true, started: true, killed: true}, "resource_limit"},
		"clean exit":            {ctx, 0, "", containerFacts{known: true, oomed: true}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			if got := resourceReason(tc.ctx, tc.code, tc.tail, tc.facts); got != tc.reason {
				t.Fatalf("reason %q want %q", got, tc.reason)
			}
		})
	}
}
