package run

// Purpose: the placement inputs of the dispatch hook (P7-AUTO-28): the run's
// world, its effective scheduling policy and the scheduler job of an attempt.
// Inputs:  the run Session, the pipeline row and the attempt being placed.
// Outputs: sched.World + digest, sched.Policy, sched.Job.
// Constraints: the job mapping is planview.SchedJobs, the one `ci plan` uses, and
// the world is world.BuildForLocal, so a plan and a run describe the same
// machine and the same registered nodes the same way (D-1042, D-0989). The
// hook's world holds registered nodes; a provider-hosted runner is dropped
// because a foreground run cannot await a provider lease, unless a coordinator
// process runs the pipeline (Session.AwaitProvider, D-1394). This file must not
// import internal/dispatch (only dispatch_hook.go may).

import (
	"context"
	"runtime"

	"github.com/nself-org/plugins/free/ci/internal/fairq"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

// platform is the local runner's platform string.
func platform() string { return runtime.GOOS + "/" + runtime.GOARCH }

// attemptBand is the priority band of an attempt: the band it declares, else the
// one its run's trigger gives (fairq.For). Every live attempt is built with
// Foreground set (the coordinator waits for it), so the flag says nothing about
// the run and is not used. Background is held back to normal until P7-SCHED-12
// can place background work: a background job today meets the interactive-
// queued and operator-idle rules of sched for its own attempt and would never run.
func attemptBand(declared, trigger string) string {
	b := fairq.For(declared, trigger, false)
	if b == fairq.Background {
		b = fairq.Normal
	}
	return b.String()
}

// schedJob maps the attempt with the mapping `ci plan` uses (planview.SchedJob), then sets what
// only a run knows: the attempt key and number, the source trust, the record flag and the local
// demand. The job's own labels, tools, isolation floor and accelerators are dropped: the local
// declaration carries no measured facts for them until P7-TRUST-08, and the exec-time isolation
// hook is the gate for them.
func (rd *runDispatch) schedJob(s AttemptSpec, id, row string, cpu, mem int) sched.Job {
	c := planview.Context{PipelineID: rd.sess.RunID, Project: rd.row.Project, Support: []string{platform()}, DefaultPlatform: platform(), Foreground: s.Foreground}
	if p := rd.sess.Plan; p != nil && len(p.Support) > 0 {
		c.Support = p.Support
	}
	j := planview.SchedJob(c, planview.PlannedJob{ID: row, Name: row, Job: *s.Job})
	j.Key, j.Attempt, j.Trust = id, s.N, model.TrustClass(rd.row.SourceTrust)
	j.Pipeline.Band = attemptBand(s.Band, rd.row.Trigger)
	j.Accelerators = nil
	j.Pipeline.Record = rd.sess.Record
	req := model.PlacementRequirements{Platform: platform(), CPU: int64(cpu), MemMB: int64(mem)}
	if s.Job.RemoteOnly { // the only jobs that go to a node with requirements: the world can check them against the node's facts
		req.Labels, req.Tools, req.IsolationMin, req.Network = j.Requirements.Labels, j.Requirements.Tools, j.Requirements.IsolationMin, j.Requirements.Network
		if s.Job.Requirements != nil {
			j.Accelerators = s.Job.Requirements.Accelerators
		}
	}
	j.Requirements = req
	if rd.sess.LocalOnly != nil && rd.sess.LocalOnly(*s.Job) { // only the runner that is this machine carries the label
		j.Requirements.Labels = append(j.Requirements.Labels, world.LocalLabel)
	}
	return j
}

// policy resolves the run's effective scheduling policy once, as PolicyHook does.
// A failure is returned and retried on the next call: a transient git or store
// error must not fail every later job of the run.
func (rd *runDispatch) policy(ctx context.Context, _ string) (sched.Policy, error) {
	rd.polM.Lock()
	defer rd.polM.Unlock()
	if rd.pol != nil {
		return *rd.pol, nil
	}
	root := rd.row.Project
	if root == "" {
		root = "."
	}
	res := policy.Resolver{Store: rd.sess.Store, Run: policy.Runner(report.Capture)}
	got, err := res.Resolve(ctx, policy.Repo{Root: root, Project: policy.ProjectIDGitCached(ctx, res.Run, root), Visibility: "unknown", Class: "owner"}, policy.Revision{Dir: root, Run: rd.sess.PolicyRun})
	if err != nil {
		return sched.Policy{}, err
	}
	p, err := schedpolicy.Placement(&got.Effective)
	if err != nil {
		return sched.Policy{}, err
	}
	qp, err := schedpolicy.Queue(&got.Effective)
	if err != nil {
		return sched.Policy{}, err
	}
	rd.pol, rd.qp = &p, qp
	return p, nil
}

// world builds the placement world of the run: this machine as "local" (probe
// facts, the PERSONAL own node declaration, docker unknown because the run loop
// does not probe for it) and every registered node.
func (rd *runDispatch) world(ctx context.Context, p sched.Pipeline) (sched.World, string, error) {
	capacity := rd.sess.Capacity()
	w, digest, err := world.BuildForLocal(ctx, world.Deps{Store: rd.sess.Store, JobIDs: rd.sess.jobRows(), Policy: rd.policy, Clock: rd.sess.now},
		world.LocalInput{Project: rd.row.Project, Platform: platform(), Capacity: capacity, DockerUnknown: true, Now: rd.sess.now()}, p)
	if err != nil {
		return sched.World{}, "", err
	}
	w, changed := dropProviderRunners(w, rd.sess.AwaitProvider)
	if !changed {
		return w, digest, nil
	}
	digest, err = world.Digest(w)
	return w, digest, err
}

// dropProviderRunners removes the provider-hosted and ephemeral runners, which a foreground run cannot
// await a lease on; with await (a coordinator process owns the run) it removes nothing. changed is false
// when there was nothing to remove (the digest stays valid).
func dropProviderRunners(w sched.World, await bool) (out sched.World, changed bool) {
	if await {
		return w, false
	}
	keep := w.Runners[:0:0]
	for _, r := range w.Runners {
		if r.Location != "hosted" && !r.Ephemeral {
			keep = append(keep, r)
		}
	}
	if len(keep) == len(w.Runners) && len(w.Hosted) == 0 {
		return w, false
	}
	w.Runners, w.Hosted = keep, nil
	return w, true
}
