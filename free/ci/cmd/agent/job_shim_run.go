package main

// Purpose: run every process job step of a system node through the job shim (D-1230). When the agent
// serves as the system agent user (user nself-ci, NSELF_CI_AGENT_HOME=/var/lib/nself-ci-agent, or
// /var/db/nself-ci-agent on macOS: the pull-mode service `nodes add --system` installs), a step of slot s starts as
// `sudo -n -u nself-ci-job-<s+1> /opt/nself-ci-agent/bin/nself-ci-agent job-shim` with the payload on
// stdin (internal/exec StepShim), so it runs under the job user's UID and cannot read the node key.
// Inputs:  the agent's environment, user and executable path (package vars, replaced by tests).
// Outputs: an Isolator wrapper, nil when the node is not a system node (today's behaviour).
// Constraints: fail closed. A failed sudo or shim is the step's failure, never a start as the agent's UID.
//          The system binary (/opt/nself-ci-agent/bin/nself-ci-agent) serving as anyone else (push mode as
//          the ssh user) refuses process jobs: the node is recorded uid-separated, so a job must never run
//          as the ssh user there. Container and VM jobs are not touched (their UID is mapped inside).

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/hygiene"
	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap"
	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap/darwin"
)

// The seams of system-mode detection and of the chain (tests replace them).
var (
	shimSudo      = "/usr/bin/sudo"
	shimAgentHome = func() string { return os.Getenv("NSELF_CI_AGENT_HOME") }
	shimUserName  = func() string {
		if u, err := user.Current(); err == nil {
			return u.Username
		}
		return ""
	}
	shimExecutable = func() string {
		exe, err := os.Executable()
		if err != nil {
			return ""
		}
		if r, err := filepath.EvalSymlinks(exe); err == nil {
			return r
		}
		return exe
	}
)

// shimEnv is the chain's own environment (sudo resets it; the job's environment is in the request).
var shimEnv = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin"}

// systemShim returns the wrapper that runs process jobs of slot through the shim (or, for the system
// binary serving outside the system agent user, refuses them), nil on a node that is not a system node.
func systemShim(slot int) func(ciexec.Isolator) ciexec.Isolator {
	if home := shimAgentHome(); (home == bootstrap.StateDir || home == darwin.StateDir) && shimUserName() == bootstrap.AgentUser {
		u := hygiene.JobUserName(slot)
		return func(inner ciexec.Isolator) ciexec.Isolator { return &shimIsolator{inner: inner, user: u} }
	}
	if shimExecutable() == bootstrap.AgentPath {
		return func(inner ciexec.Isolator) ciexec.Isolator { return &shimIsolator{inner: inner} }
	}
	return nil
}

// shimChain is the exact command line a step of user starts as.
func shimChain(user string) []string {
	return []string{shimSudo, "-n", "-u", user, bootstrap.AgentPath, "job-shim"}
}

// shimIsolator moves a process job's command into the shim request. An empty user refuses process jobs.
type shimIsolator struct {
	inner ciexec.Isolator
	user  string
}

// Prepare runs the inner isolator, then puts the chain in front of a process job.
func (i *shimIsolator) Prepare(ctx context.Context, s *ciexec.JobSpec) error {
	if err := i.inner.Prepare(ctx, s); err != nil {
		return err
	}
	if s.Plan != nil || s.Job.Isolation == "container" {
		return nil // container-mapped: the executor never starts it as a host process
	}
	if i.user == "" {
		return fmt.Errorf("E682: this is a system node and the agent serves as %q, not as %s with its system NSELF_CI_AGENT_HOME; process jobs run only through the job shim (run the system service: pull mode)",
			shimUserName(), bootstrap.AgentUser)
	}
	s.Shim = &ciexec.StepShim{Payload: s.Command, Env: append([]string{}, shimEnv...)}
	s.Command = shimChain(i.user)
	return nil
}
