package main

// Purpose: the agent's half of the job-shim workspace hand-off (D-1230). shimCall runs one housekeeping
// op as a job user through the same chain a step uses (`sudo -n -u nself-ci-job-<n> <agent> job-shim`,
// the one sudo rule the install creates), and shimStager (an internal/exec Stager) streams the attempt's
// workspace into the job user's own directory before a step and back after it.
// Inputs:  the job user, the attempt id, the agent's workspace path.
// Outputs: the directory the step runs in (<jobs>/<n>/<attempt>/ws) and the HOME and TMPDIR that point
//          into the same attempt directory; the workspace refilled with the job's outputs.
// Constraints: no new privilege: the job user creates and owns its copy, so no chown, chgrp or mode change
//          ever runs on a path, and no other job user, nor the world, can enter it (0700 under the slot
//          root of that user alone). The agent never opens the job user's files; the trees cross as
//          streams, and both ends refuse to follow links (job_shim_tree.go). The op's paths are built
//          by the shim from its own user name; the request carries only the op and the attempt id.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"strings"
)

// shimCall runs one op as user; in feeds the op after the request line and out receives its stdout.
// Tests replace it with an in-process call of runShimOp.
var shimCall = func(ctx context.Context, user string, req shimRequest, in io.Reader, out io.Writer) error {
	line, err := json.Marshal(req)
	if err != nil {
		return err
	}
	chain := shimChain(user)
	cmd := osexec.CommandContext(ctx, chain[0], chain[1:]...)
	cmd.Env = append([]string{}, shimEnv...)
	cmd.Dir = "/"
	stdin := io.Reader(bytes.NewReader(append(line, '\n')))
	if in != nil {
		stdin = io.MultiReader(stdin, in)
	}
	cmd.Stdin = stdin
	if out == nil {
		out = io.Discard
	}
	cmd.Stdout = out
	var stderr bytes.Buffer
	cmd.Stderr = &limitedWriter{w: &stderr, n: 4096}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("job-shim %s as %s: %v: %s", req.Op, user, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// limitedWriter keeps the first n bytes and drops the rest.
type limitedWriter struct {
	w io.Writer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n > 0 {
		k := min(len(p), l.n)
		_, _ = l.w.Write(p[:k])
		l.n -= k
	}
	return len(p), nil
}

// shimStager hands workspaces to one job user.
type shimStager struct{ user string }

// StageIn streams the workspace to the job user, which unpacks it into its own new attempt directory.
func (s shimStager) StageIn(ctx context.Context, attempt, workspace string) (string, []string, error) {
	dirs, err := attemptDirs(shimJobsDir(), s.user, attempt)
	if err != nil {
		return "", nil, err
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return "", nil, err
	}
	defer root.Close()
	pr, pw := io.Pipe()
	packed := make(chan error, 1)
	go func() {
		err := packTree(root, pw)
		_ = pw.CloseWithError(err)
		packed <- err
	}()
	err = shimCall(ctx, s.user, shimRequest{Op: opStageIn, Attempt: attempt}, pr, nil)
	_ = pr.CloseWithError(errors.New("job-shim stage-in ended"))
	if perr := <-packed; err == nil && perr != nil {
		err = perr
	}
	if err != nil {
		return "", nil, err
	}
	return dirs.WS, []string{"HOME=" + dirs.Home, "TMPDIR=" + dirs.Tmp}, nil
}

// StageOut empties the workspace and unpacks the job user's copy into it.
func (s shimStager) StageOut(ctx context.Context, attempt, workspace string) error {
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := emptyTree(root); err != nil {
		return err
	}
	pr, pw := io.Pipe()
	called := make(chan error, 1)
	go func() {
		err := shimCall(ctx, s.user, shimRequest{Op: opStageOut, Attempt: attempt}, nil, pw)
		_ = pw.CloseWithError(err)
		called <- err
	}()
	err = unpackTree(root, pr)
	_, _ = io.Copy(io.Discard, pr) // read to the end, so a refused tree never leaves the shim blocked on its pipe
	if cerr := <-called; cerr != nil {
		return cerr
	}
	return err
}
