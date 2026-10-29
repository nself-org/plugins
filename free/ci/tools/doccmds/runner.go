package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// blockTimeout bounds one doc block, so a hung command fails the check instead of stalling CI.
const blockTimeout = 10 * time.Minute

// shellRunner runs a block with bash in the fixture directory.
type shellRunner struct {
	timeout time.Duration
}

// Run executes script with bash in dir and returns its exit code. Block output goes to stderr so the
// failure report on stdout stays readable.
func (r shellRunner) Run(dir, script string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	cmd.Dir = dir
	cmd.Env = fixtureEnv(dir)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return -1, fmt.Errorf("timed out after %s", r.timeout)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	if err != nil {
		return -1, err
	}
	return 0, nil
}

// fixtureEnv is the whole environment a block sees. Nothing else from the caller leaks in, so a block
// cannot read a credential from the CI environment.
func fixtureEnv(dir string) []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + dir,
		"TMPDIR=" + os.TempDir(),
		"LANG=C",
		"NSELF_V15=1",
		"CGO_ENABLED=0",
		"GOFLAGS=-mod=vendor",
	}
}

// newFixture makes an empty temp project with a git repo. It returns the directory and a function that
// removes it.
func newFixture() (string, func(), error) {
	dir, err := os.MkdirTemp("", "doccmds-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	cmd := exec.Command("git", "init", "-q", dir)
	cmd.Env = fixtureEnv(dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("git init: %w: %s", err, out)
	}
	return dir, cleanup, nil
}
