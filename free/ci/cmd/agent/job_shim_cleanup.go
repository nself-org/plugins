package main

// Purpose: end-of-attempt cleanup of a job-shim attempt (D-1230). shimJobs is hygiene's JobRunner on a
// system node: it kills the job user's processes and removes the job user's attempt directory through the
// job shim, as that user, because the agent can do neither. cleanupShimAttempt runs hygiene's CleanupJob
// with it after every process attempt that went through the shim, and a failure drains this node.
// Inputs:  the job user, the attempt id and the executor home of the attempt.
// Outputs: nil when nothing of the attempt is left; an E681 error otherwise, after which the node is
//          drained: every later job is refused (E681) before anything of it runs, until the agent restarts.
// Constraints: no path from the job or from hygiene's caller reaches the shim: the shim builds the
//          attempt's directory from its own user name and the validated attempt id (job_shim_ops.go). A
//          runner asked about any UID but its job user's refuses. A cleanup that cannot finish never
//          leaves the node taking work (fail closed).

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/hygiene"
)

// shimLookupUID resolves a job user's UID (tests replace it).
var shimLookupUID = func(name string) (int, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(u.Uid)
}

const shimCleanupTimeout = 60 * time.Second

// shimJobs runs hygiene's job-user operations through the job shim.
type shimJobs struct {
	user string
	uid  int
}

func (j shimJobs) check(uid int) error {
	if uid != j.uid || uid <= 0 {
		return fmt.Errorf("job runner of %s (uid %d) asked about uid %d", j.user, j.uid, uid)
	}
	return nil
}

// KillAll kills every process of the job user, as the job user.
func (j shimJobs) KillAll(ctx context.Context, uid int) error {
	if err := j.check(uid); err != nil {
		return err
	}
	return shimCall(ctx, j.user, shimRequest{Op: opKill}, nil, nil)
}

// EmptyRoot removes the job user's directory of the attempt, as the job user. base is the agent's own
// work base: it is never sent, since the shim derives the job user's directory itself.
func (j shimJobs) EmptyRoot(ctx context.Context, uid int, _ string, attempt string) error {
	if err := j.check(uid); err != nil {
		return err
	}
	return shimCall(ctx, j.user, shimRequest{Op: opClean, Attempt: attempt}, nil, nil)
}

// nodeDrain is the node-local drain a failed shim cleanup sets.
var nodeDrain struct {
	sync.Mutex
	reason string
}

func drainNode(reason string) {
	nodeDrain.Lock()
	defer nodeDrain.Unlock()
	if nodeDrain.reason == "" {
		nodeDrain.reason = reason
		fmt.Fprintln(os.Stderr, "hygiene: node drained, no further job will run:", reason)
	}
}

// drainedError is the refusal of a job on a drained node, nil while the node takes work.
func drainedError() error {
	nodeDrain.Lock()
	defer nodeDrain.Unlock()
	if nodeDrain.reason == "" {
		return nil
	}
	return &ciexec.Error{Code: hygiene.CodeLeak, Message: "node drained after a failed job cleanup: " + nodeDrain.reason}
}

// cleanupShimAttempt kills and removes what attempt left as user, then removes the agent's root of it.
func cleanupShimAttempt(ctx context.Context, user, home, attempt string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shimCleanupTimeout)
	defer cancel()
	uid, err := shimLookupUID(user)
	if err == nil && uid <= 0 {
		err = fmt.Errorf("job user %s has uid %d", user, uid)
	}
	if err == nil {
		h := &hygiene.Host{Base: filepath.Join(home, "work"), Home: home, Runtime: noContainers{}, Jobs: shimJobs{user: user, uid: uid},
			Logf: func(format string, a ...any) { fmt.Fprintf(os.Stderr, format+"\n", a...) }}
		err = h.CleanupJob(ctx, attempt, uid)
	}
	if err != nil {
		drainNode(err.Error())
		return &ciexec.Error{Code: hygiene.CodeLeak, Message: "job cleanup failed: " + err.Error()}
	}
	return nil
}
