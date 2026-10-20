package dispatch

// Purpose: the isolation level a delivered job runs at (P7-TRUST-18, finding A01). Placement only
// filters nodes by the trust floor; the node's agent runs the job at the level written in the job
// definition it receives, so delivery raises that level to the floor placement required.
// Inputs:  a queued job and the runner it was placed on.
// Outputs: the job definition with Isolation = max(declared level, required floor).
// Constraints: a level is never lowered; an empty or unknown declared level ranks lowest
// (trust.RankIsolation), so it is replaced by the floor; a hosted delivery is left as written
// (the provider's disposable environment is its isolation, and "hosted-disposable" is not a
// level an agent can run). The floor is derived with the same trust functions sched.Place used,
// from the job's and the runner's declared facts; policy-level minimums are not visible here.

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// requiredIsolation is the lowest level the placement of q on r allows the job to run at, or ""
// when no agent-run level applies (a hosted runner).
func requiredIsolation(q Queued, r sched.Runner, hosted bool) model.Isolation {
	if hosted {
		return ""
	}
	j := q.Job.Sched
	d := r.Decl
	if len(d.Accepts) == 0 {
		d = sched.DeclarationFromCapability(r.Capability, r.CoordinatorHost, r.CoordinatorMode)
	}
	if d.PersonalOwnNode { // the same dedicated trusted case sched.Place derives (R-DX-12)
		d.PersonalOwnRun = d.PersonalOwnRun || j.PersonalOwnRun
		for _, c := range j.SecretClasses {
			d.ReleaseJob = d.ReleaseJob || c == "release"
		}
	}
	facts := trust.JobFacts{Trust: j.Trust, SecretClasses: j.SecretClasses, Network: j.Network, PersonalOwnRun: j.PersonalOwnRun, Release: j.Pipeline.Release}
	floor := trust.MinIsolation(facts, trust.RunnerFacts{Ownership: d.Ownership, Dedicated: trust.IsDedicatedTrusted(d), UIDSeparation: d.UIDSeparation,
		PersonalOwnNode: d.PersonalOwnNode, NoLANRoute: d.NoLANRoute})
	if m := j.Requirements.IsolationMin; m != "" && trust.RankIsolation(trust.DecodeIsolation(string(m))) > trust.RankIsolation(floor) {
		floor = trust.DecodeIsolation(string(m))
	}
	if floor == "hosted-disposable" { // a provider level, never an agent-run one
		return ""
	}
	return floor
}

// raiseIsolation returns job with its level raised to floor; it never lowers a level.
func raiseIsolation(job model.Job, floor model.Isolation) model.Job {
	if floor != "" && trust.RankIsolation(floor) > trust.RankIsolation(job.Isolation) {
		job.Isolation = floor
	}
	return job
}
