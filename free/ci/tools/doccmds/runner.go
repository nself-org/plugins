package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// blockTimeout bounds one doc block, so a hung command fails the check instead of stalling CI.
const blockTimeout = 10 * time.Minute

// fixtureEnvPolicy gives a block the fixture environment and nothing else.
type fixtureEnvPolicy struct{ dir string }

func (p fixtureEnvPolicy) Environment(context.Context, model.Job) ([]string, error) {
	return fixtureEnv(p.dir), nil
}

// runIn runs argv through the command funnel (internal/exec) with the fixture environment of dir.
// The command starts in the executor's empty workspace; out receives its output.
//
// timeout is a wall-clock bound carried by the context, not by JobSpec.Timeout: the funnel turns a
// JobSpec timeout into a CPU ulimit (hard limit), and a block that runs `nself ci -- cmd` starts an
// inner job whose own limit (30m by default) is then above the block's. The inner job cannot raise
// its hard limit and dies with "ulimit: Operation not permitted". Leaving the block without a CPU
// limit lets the inner job set its own, and the context deadline still fails a hung block.
func runIn(ctx context.Context, dir string, timeout time.Duration, out func([]byte), argv ...string) (ciexec.AttemptResult, error) {
	home, err := os.MkdirTemp("", "doccmds-home-")
	if err != nil {
		return ciexec.AttemptResult{}, err
	}
	defer func() { _ = os.RemoveAll(home) }()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	ex := ciexec.Executor{Isolation: ciexec.BaselineIsolator{}, Env: fixtureEnvPolicy{dir}}
	res := ex.Run(ctx, ciexec.JobSpec{
		AttemptID: "doccmds-" + strconv.FormatInt(time.Now().UnixNano(), 36), CoordinatorID: "doccmds",
		Job: model.Job{Kind: "static"}, Home: home, Command: argv, Output: out,
	})
	return res, nil
}

// shellRunner runs a block with bash in the fixture directory.
type shellRunner struct {
	timeout time.Duration
}

// Run executes script with bash in dir and returns its exit code. Block output goes to stderr so the
// failure report on stdout stays readable.
func (r shellRunner) Run(dir, script string) (int, error) {
	// $HOME is the fixture directory (fixtureEnv); the block runs there, as it would in a fresh project.
	res, err := runIn(context.Background(), dir, r.timeout, func(b []byte) { _, _ = os.Stderr.Write(b) },
		"bash", "-c", "cd \"$HOME\" || exit 1\n"+script)
	if err != nil {
		return -1, err
	}
	if res.Attempt.FailureClass == "timeout" {
		return -1, fmt.Errorf("timed out after %s", r.timeout)
	}
	if res.Err != nil {
		return -1, res.Err
	}
	return res.ExitCode, nil
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
	var out []byte
	res, err := runIn(context.Background(), dir, time.Minute, func(b []byte) { out = append(out, b...) }, "git", "init", "-q", dir)
	if err == nil && res.Err != nil {
		err = res.Err
	}
	if err == nil && res.ExitCode != 0 {
		err = fmt.Errorf("exit %d", res.ExitCode)
	}
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("git init: %w: %s", err, out)
	}
	return dir, cleanup, nil
}
