package exec

// Purpose: verify what an exit code of 125 or 137 means against the container runtime (D-0968).
// Inputs:  the runtime binary, the container name and the instant the run began.
// Outputs: containerFacts, the lifecycle events the runtime recorded for that container, and the
// resource reason derived from them.
// Constraints: planned containers run with --rm, so the container is gone by the time the exit code
// is known and `inspect` has nothing to read; the runtime's event log (`events --since --until`) is
// the record that outlives it. An empty or failed answer proves nothing: the callers then fall back
// to reading the exit code alone, as before.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// containerFacts is what the runtime's event log says about one container.
type containerFacts struct {
	known   bool // the runtime answered with at least one event for the container
	started bool // a start event: the job's process ran
	oomed   bool // an oom event: the kernel killed a process of the container for memory
	killed  bool // a kill event: the runtime was asked to signal the container (docker kill, stop, rm -f)
}

// jobRan reports whether the container's process ran, so an exit code of 125 was the job's own.
// An unanswered question is not proof either way, so it reports false (the runtime-failure reading).
func (f containerFacts) jobRan() bool { return f.known && f.started }

// askRuntime reads the lifecycle events of container name between began and now. Any failure to
// ask returns the zero value (nothing known).
func askRuntime(binary, name string, began time.Time) containerFacts {
	if binary == "" || name == "" {
		return containerFacts{}
	}
	since, until := began.Add(-time.Second), time.Now() // until must not be in the future: the runtime would wait for it
	out, err := runtimeQuery(context.WithoutCancel(context.Background()), binary, "events", "--since", unixTime(since), "--until", unixTime(until),
		"--filter", "container="+name, "--filter", "type=container", "--format", "{{json .}}")
	if err != nil {
		return containerFacts{}
	}
	var f containerFacts
	for _, line := range strings.Split(string(out), "\n") {
		var ev struct{ Action, Status string } // docker names the lifecycle step Action, podman Status
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		step := ev.Action
		if step == "" {
			step = ev.Status
		}
		f.known = true
		switch step {
		case "start":
			f.started = true
		case "oom":
			f.oomed = true
		case "kill":
			f.killed = true
		}
	}
	return f
}

func unixTime(t time.Time) string { return fmt.Sprintf("%d.%09d", t.Unix(), t.Nanosecond()) }

// resourceReason reports "resource_limit" for a SIGKILLed container the runtime did not
// contradict or output showing a refused fork (the pids limit); a deadline or cancel is never a
// resource reason by itself. The runtime contradicts it by recording a kill request for the
// container (an operator's docker kill or stop, or a sweep) with no oom event. Its absence is not
// proof: the oom event can be lost when --rm removes the container first, so exit 137 with no
// kill request on record stays "may have hit a limit", as before.
func resourceReason(ctx context.Context, code int, tail string, f containerFacts) string {
	if code == 0 {
		return ""
	}
	if code == 137 && ctx.Err() == nil && (f.oomed || !f.killed) {
		return "resource_limit"
	}
	for _, sig := range resourceSignatures {
		if strings.Contains(tail, sig) {
			return "resource_limit"
		}
	}
	return ""
}
