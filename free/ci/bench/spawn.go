package bench

// Purpose: start the processes the harness times (the nself-ci binary under
// test, go build, git for fixtures) and report their wall time.
// Inputs:  argv, working directory, environment.
// Outputs: wall time of the child, its exit code, and a short output tail.
// Constraints: this is the one place the harness starts a process, and it is a
// timing tool, not a job runner: it must not add the engine's own funnel
// (workspace copy, process-tree polling) to the time it measures, so it uses
// os/exec directly. architecture/layers_test.go and
// internal/nodes/provision/single_exec_test.go list this file by name (P7-CI-46,
// lead ruling in the CI Epic Invalidations).

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// tailLimit bounds the output kept from a child, for failure messages.
const tailLimit = 4096

// tailBuffer keeps the last tailLimit bytes written to it.
type tailBuffer struct{ b []byte }

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.b = append(t.b, p...)
	if len(t.b) > tailLimit {
		t.b = t.b[len(t.b)-tailLimit:]
	}
	return len(p), nil
}

// Timing is the outcome of one timed child.
type Timing struct {
	Wall     time.Duration
	ExitCode int
	Tail     string
}

// MS is the wall time in milliseconds.
func (r Timing) MS() float64 { return float64(r.Wall.Microseconds()) / 1000 }

// spawn runs argv in dir with env (nil env means an empty environment) and
// times it. A child that exits non-zero is a Timing with ExitCode set and a nil
// error; only a child that could not be started returns an error.
func spawn(ctx context.Context, argv []string, dir string, env []string) (Timing, error) {
	if len(argv) == 0 {
		return Timing{}, fmt.Errorf("spawn: empty command")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	var tail tailBuffer
	cmd.Stdout, cmd.Stderr = &tail, &tail
	start := time.Now()
	err := cmd.Run()
	wall := time.Since(start)
	run := Timing{Wall: wall, Tail: strings.TrimSpace(string(bytes.TrimSpace(tail.b)))}
	if err == nil {
		return run, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		run.ExitCode = exit.ExitCode()
		return run, nil
	}
	return run, err
}

// must runs argv and fails unless it exits 0.
func must(ctx context.Context, argv []string, dir string, env []string) (Timing, error) {
	r, err := spawn(ctx, argv, dir, env)
	if err != nil {
		return r, fmt.Errorf("%s: %w", strings.Join(argv, " "), err)
	}
	if r.ExitCode != 0 {
		return r, fmt.Errorf("%s: exit %d: %s", strings.Join(argv, " "), r.ExitCode, r.Tail)
	}
	return r, nil
}
