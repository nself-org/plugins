package dispatch

// Purpose: the isolation level a delivered job runs at (P7-TRUST-18, finding A01). Placement only
// filters nodes by the trust floor; the node's agent runs the job at the level written in the job
// definition it receives, so delivery raises that level to the floor placement required.
// Inputs:  a queued job and the runner it was placed on.
// Outputs: the job definition with Isolation = max(declared level, required floor).
// Constraints: a level is never lowered; an empty or unknown declared level ranks lowest
// (trust.RankIsolation), so it is replaced by the floor; a hosted delivery (sched.RunnerHosted,
// the predicate placement uses) is left as written: the provider's disposable environment is its
// isolation. "hosted-disposable" is not a level an agent can run, so on a non-hosted runner a
// minimum of that rank is delivered as ephemeral-vm (same rank), kept with the trust floor.
// The floor is derived with the same trust functions sched.Place used, from the job's and the
// runner's declared facts plus the policy minimums (override and trust.isolation_min) and the
// production boundary's vm floor (deploy host override, coordinator host).

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// requiredIsolation is the lowest level the placement of q on r allows the job to run at, or ""
// when no agent-run level applies (a hosted runner). pol carries the policy-level minimums
// (sched.overrides.min_isolation and trust.isolation_min), as placement applied them.
func requiredIsolation(q Queued, r sched.Runner, pol sched.Policy) model.Isolation {
	if sched.RunnerHosted(r) {
		return ""
	}
	j := q.Job.Sched
	d := declarationFor(j, r)
	facts := trust.JobFacts{Trust: j.Trust, SecretClasses: j.SecretClasses, Network: j.Network, PersonalOwnRun: j.PersonalOwnRun, Release: j.Pipeline.Release}
	floor := trust.MinIsolation(facts, trust.RunnerFacts{Ownership: d.Ownership, Dedicated: trust.IsDedicatedTrusted(d), UIDSeparation: d.UIDSeparation,
		PersonalOwnNode: d.PersonalOwnNode, NoLANRoute: d.NoLANRoute})
	policyMin := policyIsolation(pol.Trust)
	// The production boundary (D21, D28): admission only checks the node's declared level; the job itself must run at vm there.
	for _, m := range []model.Isolation{j.Requirements.IsolationMin, pol.Overrides.MinIsolation, policyMin, trust.BoundaryMinIsolation(facts, d)} {
		if m == "" {
			continue
		}
		if m = trust.DecodeIsolation(string(m)); m == "hosted-disposable" {
			m = "ephemeral-vm" // same rank; a provider level is never an agent-run one
		}
		if trust.RankIsolation(m) > trust.RankIsolation(floor) {
			floor = m
		}
	}
	if floor == "hosted-disposable" { // an untrusted floor computed for a hosted runner
		floor = "ephemeral-vm"
	}
	return floor
}

// declarationFor is the declaration placement judged runner r by for job j: the runner's own, else the one its
// capability states, with the coordinator facts and the dedicated trusted case (R-DX-12) as sched.Place derives them.
func declarationFor(j sched.Job, r sched.Runner) trust.Declaration {
	d := r.Decl
	if len(d.Accepts) == 0 {
		d = sched.DeclarationFromCapability(r.Capability, r.CoordinatorHost, r.CoordinatorMode)
	}
	d.CoordinatorHost, d.CoordinatorMode = r.CoordinatorHost, r.CoordinatorMode // as sched eligibility sets them
	if d.PersonalOwnNode {
		d.PersonalOwnRun = d.PersonalOwnRun || j.PersonalOwnRun
		for _, c := range j.SecretClasses {
			d.ReleaseJob = d.ReleaseJob || c == "release"
		}
	}
	return d
}

// runnerFacts is what the secret gate knows of the runner a lease goes to.
func runnerFacts(j sched.Job, r sched.Runner) trust.RunnerFacts {
	d := declarationFor(j, r)
	return trust.RunnerFacts{Ownership: d.Ownership, Dedicated: trust.IsDedicatedTrusted(d), Hosted: sched.RunnerHosted(r), UIDSeparation: d.UIDSeparation,
		PersonalOwnNode: d.PersonalOwnNode, NoLANRoute: d.NoLANRoute, Isolation: d.Isolation}
}

// policyIsolation reads the effective trust.isolation_min, as trust.Admissible does.
func policyIsolation(e trust.Effective) model.Isolation {
	v, _, _ := e.Value("trust.isolation_min")
	s, _ := v.(string)
	return model.Isolation(s)
}

// raiseIsolation returns job with its level raised to floor; it never lowers a level.
func raiseIsolation(job model.Job, floor model.Isolation) model.Job {
	if floor != "" && trust.RankIsolation(floor) > trust.RankIsolation(job.Isolation) {
		job.Isolation = floor
	}
	return job
}
