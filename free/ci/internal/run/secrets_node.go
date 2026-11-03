package run

// Purpose: route a TEAM job that declares secrets to a leased node (D-1534). The coordinator passes
// references only (secrets_team.go), so such a job cannot run on the coordinator's own machine; it runs on
// a secrets-eligible node that resolves the references itself, under its own age identity, inside the job.
// Inputs:  the run's Options and one job.
// Outputs: the job as the leased node must run it, and whether it must run on a node.
// Constraints: only a TEAM run and only a job with secrets. A job that has another reason to stay on this
// machine (release kind, at-most-once key, cache, artifacts, output hooks, services, --network or --service)
// stays here, where the TEAM Source refuses it with E677: fail closed, never a value on the coordinator.
// The invocation's controls a node does not receive are carried in the job itself: --container raises the
// job's isolation to at least container (dispatch never lowers it) and --timeout caps the job's timeout.
// The release.start check already ran at dispatch (cmd serve); the node checks trust class and its grant.

import (
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// nodeSecretJob reports whether job must run on a leased node because it reads secrets in a TEAM run,
// and returns the job with the run's container and timeout controls folded in.
func (rs *runState) nodeSecretJob(job model.Job) (model.Job, bool) {
	o := rs.c.Opts
	if !o.TeamRun || len(job.Secrets) == 0 {
		return job, false
	}
	if o.Network != "" || len(o.Services) > 0 || rs.wantsOutput(job) || declares(job) && !job.RemoteOnly || job.Kind == "release" ||
		idempotencyKey(&job, "x") != "" || job.Cache != nil || len(job.NeedsServices) > 0 {
		return job, false
	}
	return nodeJobControls(job, o.Container, o.Timeout), true
}

// nodeJobControls folds --container and --timeout into a job a node runs. It never lowers an isolation
// level and never lengthens a timeout.
func nodeJobControls(job model.Job, container bool, timeout time.Duration) model.Job {
	if container && trust.RankIsolation(job.Isolation) < trust.RankIsolation("container") {
		job.Isolation = "container"
	}
	if timeout > 0 {
		if d, err := time.ParseDuration(job.Timeout); err != nil || d <= 0 || d > timeout {
			job.Timeout = timeout.String()
		}
	}
	return job
}

// nodeJob is the job a dispatched attempt carries: the session's node form when the job must run on a
// leased node, else the job unchanged. remote is true for the former.
func nodeJob(sess *Session, job model.Job) (out model.Job, remote bool) {
	if sess == nil || sess.NodeJob == nil {
		return job, false
	}
	return sess.NodeJob(job)
}
