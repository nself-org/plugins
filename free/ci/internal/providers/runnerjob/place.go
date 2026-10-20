package runnerjob

// Purpose: Constrain, the one statement of what the scheduler must believe about a runner job (Epic D6).
// Inputs:  the sched.Job the planner mapped from the pipeline's job (planview.SchedJob).
// Outputs: the same job pinned to untrusted work, the restricted network scope, no secret classes,
//          remote only, the isolation floor sandboxed-container and the background band.
// Constraints: the model job has no network field and planview defaults the scheduling scope to
//          internet; a forge-runner job must be placed as untrusted-receiving whatever that default says
//          (a repo-scope runner is never governed by a runner group, so no demand is proven safe, D6).
//          The trust class itself is TRUST-13's call at dispatch; Constrain only ever lowers it.

import "github.com/nself-org/plugins/free/ci/internal/sched"

// Constrain pins j to the placement rules of a forge-runner job.
func Constrain(j *sched.Job) {
	j.Trust = "untrusted"
	j.Network = "restricted"
	j.SecretClasses = nil
	j.RemoteOnly = true
	j.PersonalOwnRun = false
	j.Requirements.IsolationMin = Isolation
	j.Requirements.Network = "restricted"
	j.Pipeline.Band = Band
}
