package run

// Purpose: the decisions around a remote attempt that are not the await itself (P7-AUTO-28): which
// jobs stay on this machine, which carrier errors count against a node, and the shared settlement of
// a finalizing attempt.
// Inputs:  a job, an error, an attempt id.
// Outputs: booleans and the settled JobOutcome.
// Constraints: see remote_await.go.

import (
	"context"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// nodeFaults are the transport reasons that are the node's doing (they count against its breaker). A
// bundle that cannot be made, a driver nobody configured, a refused secret and any store or run error
// of ours are not on the list.
var nodeFaults = map[string]bool{"transport.ssh_session_lost": true, "transport.ssh_connect_failed": true, "transport.ssh_connect_timeout": true,
	"transport.ssh_host_key_unknown": true, "transport.ssh_host_key_changed": true, "transport.ssh_handshake": true, "transport.ssh_preamble": true,
	"transport.ssh_lease_refused": true}

// nodeFault says whether a carrier error is the node's doing. The reason is the second word of an ssh
// transport error ("E656 transport.ssh_session_lost: ..."), a position the message text cannot reach.
func nodeFault(err error) bool {
	f := strings.Fields(err.Error())
	return len(f) >= 2 && strings.HasPrefix(f[0], "E65") && nodeFaults[strings.TrimSuffix(f[1], ":")]
}

// finalizePassed ends a finalizing attempt whose job passed: the finalize hooks run
// (results before artifacts), then it becomes passed. The local path and a remote
// result share it.
func (rs *runState) finalizePassed(id string, o JobOutcome, info AttemptInfo) JobOutcome {
	if ferr := rs.attemptFinalize(info); ferr != nil { // a hook refused: the attempt cannot pass
		return rs.settleFinalizeFailed(id, o, ferr)
	}
	to, err := rs.settle(id, "finalizing", "passed", "")
	if err != nil {
		return failed(o, err)
	}
	bg, done := background(context.Background())
	defer done()
	if state, _, e := rs.sess.Store.AttemptStateEpoch(bg, id); e == nil {
		to = state
	}
	switch to {
	case "passed":
		o.State = StatePassed
	case "cancelled":
		return cancelled(o, "", "cancelled by the operator")
	default:
		o.State, o.FailureClass, o.Detail = StateFailed, "infra", "a declared artifact was not committed"
	}
	return o
}

// toLost makes sure the attempt is lost: a remote attempt the dispatcher already marked lost
// stays so; a running one is moved.
func (rs *runState) toLost(id string) bool {
	bg, done := background(context.Background())
	defer done()
	if state, _, err := rs.sess.Store.AttemptStateEpoch(bg, id); err == nil && state == "lost" {
		return true
	}
	return rs.trans(id, "running", "lost", "infra") == nil
}

// localOnly says a job must run on this machine because the remote path cannot honour what the
// local one does: outcome hooks that read the output (drill), declared and implicit artifacts, caches (no
// intake from a node yet), release and at-most-once jobs (their idempotency key and deploy checks run
// in prepare), secrets (no value is sent to a node), services, requirements the world
// cannot verify (labels, tools, an isolation floor, a container: the local runner reports no
// measured facts for them) unless the job is remote-only, and the invocation's own controls
// (--container, --network, --service, --timeout), which the agent never receives.
func (rs *runState) localOnly(job model.Job) bool {
	o := rs.c.Opts
	if o.Container || o.Network != "" || o.Timeout > 0 || len(o.Services) > 0 {
		return true
	}
	if rs.wantsOutput(job) || declares(job) || job.Kind == "release" || idempotencyKey(&job, "x") != "" || job.Cache != nil || len(job.Secrets) > 0 || len(job.NeedsServices) > 0 {
		return true
	}
	if job.RemoteOnly {
		return false
	}
	r := job.Requirements
	// the planner sets isolation "process" on an ordinary job: only a stricter floor is a requirement
	return r != nil && (len(r.Labels) > 0 || len(r.Tools) > 0 || len(r.Accelerators) > 0) || job.Isolation != "" && job.Isolation != "process" || job.Container != nil
}
