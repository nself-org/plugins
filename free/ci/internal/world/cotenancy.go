package world

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// Purpose: feed the two trust inputs that no store column carries directly (findings A05, A04).
//   - Runner.Decl.Running (TM-11 co-tenancy): what is leased or running on each node right now.
//   - World.PreviousIsolation (TM-08 retry floor): the isolation level the previous attempt ran at.
// Inputs:  the active attempts of one store snapshot, the jobs the caller is placing, the built runners.
// Outputs: Running set on every runner; a floor per job key.
// Constraints: pure, no I/O. The store records no trust class, secret classes or isolation per attempt,
//   so an active attempt whose job is not among the jobs being placed is UNKNOWN and decodes to the most
//   restrictive reading (untrusted and secret-bearing). A known job reads from its own declaration.

// attemptKind is what one active attempt contributes to its node's Running.
type attemptKind struct{ untrusted, secretBearing bool }

// jobKind reads a job's co-tenancy kind the way trust.Admissible reads a job: an unknown trust class
// decodes to untrusted and any secret class other than "none" makes the job secret-bearing.
func jobKind(j sched.Job) attemptKind {
	k := attemptKind{untrusted: trust.DecodeTrust(string(j.Trust)) == "untrusted"}
	for _, c := range j.SecretClasses {
		if c != "none" {
			k.secretBearing = true
		}
	}
	return k
}

// fillRunning sets Decl.Running on each runner from the attempts leased to it. Active attempts of
// jobs the caller is not placing are unknown, so they count as untrusted and secret-bearing.
func fillRunning(runners []sched.Runner, active []store.SchedAttempt, jobs []sched.Job) {
	known := make(map[string]attemptKind, len(jobs))
	for _, j := range jobs {
		known[j.ID] = jobKind(j)
	}
	byRunner := map[string]trust.Running{}
	for _, a := range active {
		if a.RunnerID == "" {
			continue
		}
		k, ok := known[a.JobID]
		if !ok {
			k = attemptKind{untrusted: true, secretBearing: true}
		}
		r := byRunner[a.RunnerID]
		r.Untrusted = r.Untrusted || k.untrusted
		r.SecretBearing = r.SecretBearing || k.secretBearing
		byRunner[a.RunnerID] = r
	}
	for i := range runners {
		runners[i].Decl.Running = byRunner[runners[i].ID]
	}
}

// previousIsolation returns, per job key, the isolation of the runner the job's previous attempt ran
// on, as that runner declares it now. It is the retry floor input (sched.World.PreviousIsolation). A
// previous runner that is no longer in the world gives no floor; a job with no previous attempt gets none.
func previousIsolation(jobs []sched.Job, previousByJob map[string]string, runners ...[]sched.Runner) map[string]model.Isolation {
	declared := map[string]model.Isolation{}
	for _, list := range runners {
		for _, r := range list {
			declared[r.ID] = r.Decl.Isolation
		}
	}
	var out map[string]model.Isolation
	for _, j := range jobs {
		iso, ok := declared[previousByJob[j.ID]]
		if !ok || iso == "" {
			continue
		}
		if out == nil {
			out = map[string]model.Isolation{}
		}
		out[j.Key] = iso
	}
	return out
}
