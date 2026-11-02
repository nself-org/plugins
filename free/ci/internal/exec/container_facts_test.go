package exec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		"nokillrecord":  {[]string{"create", "start", "die"}, ""}, // the exit code alone is no host evidence
		"runtimesilent": {nil, ""},                                // no answer proves nothing, so no limit is claimed
	} {
		t.Run(name, func(t *testing.T) {
			fakeRuntime(t, "", "137")
			fakeEvents(t, tc.events...)
			res := (&Executor{Isolation: BaselineIsolator{}}).Run(context.Background(), plannedSpec(t, "plan-137-"+name, hardPlanner{}))
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
		res := (&Executor{Isolation: BaselineIsolator{}}).Run(context.Background(), plannedSpec(t, "plan-125-job", hardPlanner{}))
		if res.Err != nil || res.ExitCode != 125 || res.Attempt.FailureClass != "code" {
			t.Fatalf("a job that ran and exited 125 is a code failure, not a runtime one: %+v", res)
		}
	})
	t.Run("runtime failed", func(t *testing.T) {
		fakeRuntime(t, "", "125")
		fakeEvents(t, "create", "destroy") // created, never started
		res := (&Executor{Isolation: BaselineIsolator{}}).Run(context.Background(), plannedSpec(t, "plan-125-rt", hardPlanner{}))
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
	none := pidsReading{Path: "/c", Limit: "512", Reads: 9}
	refused := pidsReading{Max: 3, Path: "/c", Limit: "512", Reads: 9}
	memFull := pidsReading{MemMax: 4, Path: "/c", Limit: "512", Reads: 9}
	oomKill := pidsReading{MemMax: 4, OOMKill: 1, Path: "/c", Limit: "512", Reads: 9}
	ran := containerFacts{known: true, started: true}
	opKill := containerFacts{known: true, started: true, killed: true}
	for name, tc := range map[string]struct {
		ctx    context.Context
		code   int
		tail   string
		facts  containerFacts
		cg     pidsReading
		reason string
	}{
		"137 oom event":                           {ctx, 137, "", containerFacts{known: true, started: true, oomed: true}, none, "resource_limit"},
		"137 nothing known":                       {ctx, 137, "", containerFacts{}, pidsReading{Problem: "container not seen"}, ""},
		"137 cgroup read, no counter":             {ctx, 137, "", ran, none, ""},
		"137 memory limit counter":                {ctx, 137, "", ran, memFull, "resource_limit"},
		"137 pids limit counter":                  {ctx, 137, "", ran, refused, "resource_limit"},
		"137 operator kill, memory counter":       {ctx, 137, "", opKill, memFull, ""},
		"137 operator kill, oom_kill":             {ctx, 137, "", opKill, oomKill, "resource_limit"},
		"137 cancelled":                           {cancelled, 137, "", containerFacts{known: true, oomed: true}, oomKill, ""},
		"job prints can't fork, no host evidence": {ctx, 2, "sh: can't fork", ran, none, ""},
		"job prints can't fork, no cgroup read":   {ctx, 2, "sh: can't fork", ran, pidsReading{Problem: "container not seen"}, ""},
		"job prints can't fork, memory only":      {ctx, 2, "sh: can't fork", ran, memFull, ""},
		"refused fork with pids counter":          {ctx, 2, "sh: can't fork", ran, refused, "resource_limit"},
		"pids counter, ordinary failure output":   {ctx, 1, "FAIL: TestX", ran, refused, ""},
		"137 operator kill, fork tail":            {ctx, 137, "Cannot fork", opKill, refused, "resource_limit"},
		"clean exit":                              {ctx, 0, "", containerFacts{known: true, oomed: true}, oomKill, ""},
	} {
		t.Run(name, func(t *testing.T) {
			if got := resourceReason(tc.ctx, tc.code, tc.tail, tc.facts, tc.cg); got != tc.reason {
				t.Fatalf("reason %q want %q", got, tc.reason)
			}
		})
	}
}

// The executor reads the container cgroup itself: a refused fork is a limit only when the host's
// pids.events counter rose, whatever the job printed (D-1580).
func TestPlannedForkRefusalNeedsTheHostCounter(t *testing.T) {
	for name, tc := range map[string]struct {
		id, events string
		reason     string
	}{
		"host counter rose":   {"rose", "max 7\n", "resource_limit"},
		"job forged the line": {"forged", "max 0\n", ""},
	} {
		t.Run(name, func(t *testing.T) {
			root, proc := t.TempDir(), t.TempDir()
			oldRoot, oldProc := cgroupRoot, procRoot
			cgroupRoot, procRoot = root, proc
			t.Cleanup(func() { cgroupRoot, procRoot = oldRoot, oldProc })
			scope := filepath.Join(root, "system.slice", "docker-abc.scope")
			if err := os.MkdirAll(scope, 0700); err != nil {
				t.Fatal(err)
			}
			for file, body := range map[string]string{"pids.max": "512\n", "pids.events": tc.events, "memory.events": "max 0\noom_kill 0\n"} {
				if err := os.WriteFile(filepath.Join(scope, file), []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			fakeRuntime(t, "sh: can't fork\n", "2")
			for file, body := range map[string]string{"inspect": "abc 0\n", "cgroup": scope} {
				if err := os.WriteFile(filepath.Join(fakeRuntimeDir, file), []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			res := (&Executor{Isolation: BaselineIsolator{}}).Run(context.Background(), plannedSpec(t, "plan-fork-"+tc.id, hardPlanner{}))
			if res.Reason != tc.reason {
				t.Fatalf("reason %q want %q (%+v)", res.Reason, tc.reason, res)
			}
			if _, err := os.Stat(scope); !os.IsNotExist(err) {
				t.Fatalf("container removal must remove its cgroup, got %v", err)
			}
		})
	}
}

func TestPidsWatchAcknowledgesSampleAndRetainsFinalEvidence(t *testing.T) {
	root, proc := t.TempDir(), t.TempDir()
	oldRoot, oldProc := cgroupRoot, procRoot
	cgroupRoot, procRoot = root, proc
	t.Cleanup(func() { cgroupRoot, procRoot = oldRoot, oldProc })
	scope := fixtureCgroup(t, root, "system.slice/docker-abc.scope", "512")
	if err := os.WriteFile(filepath.Join(scope, "memory.events"), []byte("max 0\noom_kill 0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fakeRuntime(t, "", "0")
	if err := os.WriteFile(filepath.Join(fakeRuntimeDir, "inspect"), []byte("abc 0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	w := startPidsWatch("docker", "nself-ci-sample")
	t.Cleanup(func() { w.stop() })
	deadline := time.NewTimer(500 * time.Millisecond)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for w.snapshot().Reads == 0 {
		select {
		case <-deadline.C:
			t.Fatalf("no acknowledged host sample: %+v", w.snapshot())
		case <-tick.C:
		}
	}
	// Join the background worker before changing the final counter: only stop's
	// explicit last read can observe this refusal.
	w.cancel()
	w.wg.Wait()
	before := w.snapshot()
	if err := os.WriteFile(filepath.Join(scope, "pids.events"), []byte("max 7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	final := w.stop()
	if final.Max != 7 || final.Reads != before.Reads+1 {
		t.Fatalf("final host sample %+v, acknowledged sample %+v", final, before)
	}
}
