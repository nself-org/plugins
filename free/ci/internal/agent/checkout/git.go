// Package checkout moves a job's source tree to a node by content: the
// coordinator packs the exact revision into a git bundle (Create), the bundle
// travels as chunk frames on the checkout stream, and the agent receives it,
// verifies its sha256 against the lease before any git process reads it, and
// stages a detached checkout of that revision in the job root (Receiver).
//
// Purpose: carrier A source delivery (P7-NODE-03, Epic D4/D8).
// Inputs:  a repository root and revision (coordinator); chunk frames and the
// lease (agent).
// Outputs: a bundle file and its digest; a staged source directory.
// Constraints: every process runs through internal/exec (DefaultRunner); the
// bundle is never trusted by name or size alone; git runs with hooks off,
// fsck on and no system or global config; nothing here writes outside the
// directories it is given.
package checkout

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Runner runs argv and returns its combined output and exit code. err is set
// only when the process could not be run.
type Runner func(ctx context.Context, argv ...string) (out string, exit int, err error)

var runSeq atomic.Uint64

// gitTimeout bounds one git process (a bundle of a large repository).
const gitTimeout = 15 * time.Minute

// DefaultRunner runs argv through the internal/exec funnel.
func DefaultRunner(ctx context.Context, argv ...string) (string, int, error) {
	home, err := os.MkdirTemp("", "nself-ci-git-")
	if err != nil {
		return "", -1, err
	}
	defer func() { _ = os.RemoveAll(home) }()
	var buf strings.Builder
	var ex ciexec.Executor
	res := ex.Run(ctx, ciexec.JobSpec{
		AttemptID: "git-" + strconv.FormatInt(time.Now().UnixNano(), 36) + "-" + strconv.FormatUint(runSeq.Add(1), 10), CoordinatorID: "checkout",
		Job: model.Job{Kind: "static"}, Home: home, Timeout: gitTimeout, Command: argv,
		Output: func(b []byte) {
			if buf.Len() < 64<<10 {
				buf.Write(b)
			}
		},
	})
	return buf.String(), res.ExitCode, res.Err
}

// git runs `git <args>` with the hardening every checkout call shares.
func git(ctx context.Context, run Runner, args ...string) (string, error) {
	argv := append([]string{"git",
		"-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", "-c", "core.fsync=none",
		"-c", "transfer.fsckObjects=true", "-c", "fetch.fsckObjects=true", "-c", "protocol.allow=never",
		"-c", "protocol.file.allow=user", "-c", "submodule.recurse=false", "-c", "advice.detachedHead=false",
	}, args...)
	out, exit, err := run(ctx, argv...)
	if err != nil {
		return out, err
	}
	if exit != 0 {
		return out, &GitError{Args: args, Exit: exit, Output: firstLines(out)}
	}
	return strings.TrimSpace(out), nil
}

// GitError is a git process that exited non-zero.
type GitError struct {
	Args   []string
	Exit   int
	Output string
}

func (e *GitError) Error() string {
	return "git " + e.Args[0] + " failed (exit " + strconv.Itoa(e.Exit) + "): " + e.Output
}

func firstLines(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 400 {
		s = s[:400]
	}
	return s
}
