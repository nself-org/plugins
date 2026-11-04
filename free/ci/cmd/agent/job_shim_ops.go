package main

// Purpose: the job user's own housekeeping through `job-shim` (D-1230), run by the agent through the same
// sudo rule as a step: stage-in (create the attempt's directory and unpack the workspace into it),
// stage-out (pack it back), clean (remove the attempt's directory) and kill (SIGKILL every process of
// this job user).
// Inputs:  the request ("op", "attempt"), the job user's own name, stdin (stage-in's tree), stdout
//          (stage-out's tree).
// Outputs: <jobs>/<n>/<attempt>/{ws,home,tmp}, all 0700 and owned by the job user, where <jobs>/<n> is
//          the slot root the install created for nself-ci-job-<n> (0700, that user's) inside a root-owned
//          directory; an error for anything refused.
// Constraints: every path is built here from constants, the shim's own user name and a validated
//          attempt id: no path comes from the request, the agent or the job. The slot root must be a
//          plain 0700 directory this user owns (never a link); every step below it goes through an
//          os.Root, so a link the job planted cannot redirect a write, a read or the removal (hygiene's
//          confined removal). An existing attempt directory is refused, never reused. Kill never runs
//          as root (shimUserOK already refused root; shimKillAll checks again).

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/hygiene"
	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap"
	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap/darwin"
)

// The shim ops' seams (tests point them at a temporary tree and a recorder).
var (
	shimJobsDir = func() string {
		if runtime.GOOS == "darwin" {
			return darwin.JobsDir
		}
		return bootstrap.JobsDir
	}
	shimKillAll = killOwnProcesses
)

// The housekeeping ops a request may name.
const (
	opStageIn  = "stage-in"
	opStageOut = "stage-out"
	opClean    = "clean"
	opKill     = "kill"
)

// checkShimOp validates an op request: a known op, no argv, dir or env, and an attempt id where one is needed.
func checkShimOp(req shimRequest) error {
	switch req.Op {
	case opStageIn, opStageOut, opClean, opKill:
	default:
		return errors.New("bad job request: unknown op")
	}
	if len(req.Argv) != 0 || req.Dir != "" || len(req.Env) != 0 {
		return errors.New("bad job request: an op carries no argv, dir or env")
	}
	if req.Op == opKill {
		if req.Attempt != "" {
			return errors.New("bad job request: kill takes no attempt")
		}
		return nil
	}
	if !hygiene.ValidAttempt(req.Attempt) {
		return errors.New("bad job request: invalid attempt id")
	}
	return nil
}

// jobAttempt is where one attempt of a slot user lives.
type jobAttempt struct {
	Root, Dir, WS, Home, Tmp string
}

// attemptDirs builds an attempt's paths from the jobs directory, the job user's name and the attempt id.
func attemptDirs(jobs, user, attempt string) (jobAttempt, error) {
	n, ok := strings.CutPrefix(user, bootstrap.AgentUser+"-job-")
	if !ok || !jobUserName.MatchString(user) || !hygiene.ValidAttempt(attempt) || !filepath.IsAbs(jobs) {
		return jobAttempt{}, fmt.Errorf("no job directory for user %q and attempt %q", user, attempt)
	}
	root := filepath.Join(jobs, n)
	dir := filepath.Join(root, attempt)
	return jobAttempt{Root: root, Dir: dir, WS: filepath.Join(dir, "ws"), Home: filepath.Join(dir, "home"), Tmp: filepath.Join(dir, "tmp")}, nil
}

// openSlotRoot opens the slot root after checking it is a plain 0700 directory this process owns.
func openSlotRoot(path string) (*os.Root, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || !ownedBySelf(info) {
		return nil, fmt.Errorf("%s is not a 0700 directory of this job user", path)
	}
	return os.OpenRoot(path)
}

// runShimOp performs one housekeeping op as the job user user.
func runShimOp(req shimRequest, user string, in io.Reader, out io.Writer) error {
	if err := checkShimOp(req); err != nil {
		return err
	}
	if req.Op == opKill {
		return shimKillAll()
	}
	a, err := attemptDirs(shimJobsDir(), user, req.Attempt)
	if err != nil {
		return err
	}
	if req.Op == opClean {
		r, err := openSlotRoot(a.Root)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		_ = r.Close()
		return hygiene.RemoveConfined(a.Root, req.Attempt, nil)
	}
	root, err := openSlotRoot(a.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	ws := filepath.Join(req.Attempt, "ws")
	if req.Op == opStageIn {
		if err := root.Mkdir(req.Attempt, 0o700); err != nil {
			return fmt.Errorf("attempt directory: %w", err)
		}
		for _, d := range []string{ws, filepath.Join(req.Attempt, "home"), filepath.Join(req.Attempt, "tmp")} {
			if err := root.Mkdir(d, 0o700); err != nil {
				return err
			}
		}
	}
	tree, err := root.OpenRoot(ws)
	if err != nil {
		return err
	}
	defer tree.Close()
	if req.Op == opStageIn {
		return unpackTree(tree, in)
	}
	return packTree(tree, out)
}
