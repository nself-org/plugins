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
// process runs the pipeline, which keeps the hosted runners of the pipeline's own repository only
// (Session.ProviderRepo, D-1394). This file must not
// import internal/dispatch (only dispatch_hook.go may).

import (
	"context"
	"runtime"
	"slices"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/fairq"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/sched"
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

// wantPlatform is the platform an attempt asks the scheduler for: the one its job or matrix entry names
// explicitly (AttemptSpec.Platform), else the invoking host's own. An explicit platform steers placement
// to a node of that platform; with none the host's platform stays the default. A job nobody can take
// (no online node of that platform) is unplaceable with platform.mismatch on every alternative, which
// `ci explain` shows; it never runs on the wrong architecture.
func wantPlatform(explicit string) string {
	if explicit != "" {
		return explicit
	}
	return platform()
}

// schedJob maps the attempt with the mapping `ci plan` uses (planview.SchedJob), then sets what
// only a run knows: the attempt key and number, the source trust, the record flag and the local
// demand. The job's own labels, tools, isolation floor and accelerators are dropped: the local
// declaration carries no measured facts for them until P7-TRUST-08, and the exec-time isolation
// hook is the gate for them.
func (rd *runDispatch) schedJob(s AttemptSpec, id, row string, cpu, mem int) sched.Job {
	want := wantPlatform(s.Platform)
	c := planview.Context{PipelineID: rd.sess.RunID, Project: rd.row.Project, Support: []string{platform()}, DefaultPlatform: platform(), Foreground: s.Foreground}
	if p := rd.sess.Plan; p != nil && len(p.Support) > 0 {
		c.Support = p.Support
	}
	if !slices.Contains(c.Support, want) { // an explicit platform outside the host-only default support is still the job's own ask
		c.Support = append(slices.Clone(c.Support), want)
	}
	j := planview.SchedJob(c, planview.PlannedJob{ID: row, Name: row, Platform: s.Platform, Job: *s.Job})
	j.Key, j.Attempt, j.Trust = id, s.N, model.TrustClass(rd.row.SourceTrust)
	if rd.sess.AdmitTrust != "" { // the row records the requested class; placement uses the class the run was admitted at
		j.Trust = rd.sess.AdmitTrust
	}
	j.User, j.Team = rd.sess.User, rd.sess.Team
	j.Pipeline.Band = attemptBand(s.Band, rd.row.Trigger)
	j.Accelerators = nil
	j.Pipeline.Record = rd.sess.Record
	req := model.PlacementRequirements{Platform: want, CPU: int64(cpu), MemMB: int64(mem)}
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

// policy is the run's effective scheduling policy: the one the coordinator resolved (Session.Placement),
// else resolved once here as PolicyHook does.
// A failure is returned and retried on the next call: a transient git or store
// error must not fail every later job of the run.
func (rd *runDispatch) policy(ctx context.Context, _ string) (sched.Policy, error) {
	rd.polM.Lock()
	defer rd.polM.Unlock()
	if rd.pol != nil {
		return *rd.pol, nil
	}
	if sp := rd.sess.Placement; sp != nil && rd.sess.Queue != nil { // the coordinator's one resolution: the digest it recorded describes this policy (D-1087)
		rd.pol, rd.qp = sp, *rd.sess.Queue
		return *sp, nil
	}
	root := rd.row.Project
	if root == "" {
		root = "."
	}
	res := policy.Resolver{Store: rd.sess.Store, Run: policy.Runner(report.Capture)}
	got, err := res.Resolve(ctx, policy.Repo{Root: root, Project: policy.ProjectIDGitCached(ctx, res.Run, root), Visibility: policyVisibility(rd.sess.ProviderRepo), Class: classOrOwner(rd.sess.AdmitTrust)}, policy.Revision{Dir: root, Run: rd.sess.PolicyRun})
	if err != nil {
		return sched.Policy{}, err
	}
	p, qp, err := schedulingOf(&got.Effective)
	if err != nil {
		return sched.Policy{}, err
	}
	rd.pol, rd.qp = &p, qp
	return p, nil
}

// world builds the placement world of the run: this machine as "local" (probe
// facts, the PERSONAL own node declaration, docker and tools as the run's plan
// measured them, else docker unknown) and every registered node.
func (rd *runDispatch) world(ctx context.Context, p sched.Pipeline) (sched.World, string, error) {
	capacity := rd.sess.Capacity()
	in := world.LocalInput{Project: rd.row.Project, Platform: platform(), Capacity: capacity, DockerUnknown: true, Now: rd.sess.now()}
	if pl := rd.sess.Plan; pl != nil && pl.World.Measured { // the plan's measured facts, so `ci plan` and the run agree on this machine (D-1104)
		in.Docker, in.Tools, in.DockerUnknown = pl.World.Docker, pl.World.Tools, false
	}
	w, digest, err := world.BuildForLocal(ctx, world.Deps{Store: rd.sess.Store, JobIDs: rd.sess.jobRows(), Policy: rd.policy, Clock: rd.sess.now}, in, p)
	if err != nil {
		return sched.World{}, "", err
	}
	w, changed := dropProviderRunners(w, rd.sess.ProviderRepo)
	if !changed {
		return w, digest, nil
	}
	digest, err = world.Digest(w)
	return w, digest, err
}

// repoLabel is the label a hosted runner carries for the repository it dispatches to
// (hosted.LabelRepo; this package may not import the provider).
const repoLabel = "nself.repo="

// dropProviderRunners removes the provider-hosted and ephemeral runners, which a foreground run cannot
// await a lease on. With repo (a coordinator-owned run) the hosted runners labelled for that repository
// stay: a runner of another repository would receive this job's spec and revision. changed is false when
// there was nothing to remove (the digest stays valid).
func dropProviderRunners(w sched.World, repo string) (out sched.World, changed bool) {
	keep := func(rs []sched.Runner) []sched.Runner {
		kept := rs[:0:0]
		for _, r := range rs {
			switch {
			case r.Ephemeral:
			case r.Location != "hosted":
				kept = append(kept, r)
			case repo != "" && hostedFor(r, repo):
				kept = append(kept, r)
			}
		}
		return kept
	}
	runners, hostedRunners := keep(w.Runners), keep(w.Hosted)
	if len(runners) == len(w.Runners) && len(hostedRunners) == len(w.Hosted) {
		return w, false
	}
	w.Runners, w.Hosted = runners, hostedRunners
	return w, true
}

// hostedFor reports whether the hosted runner dispatches to repo ("owner/name", compared without case).
func hostedFor(r sched.Runner, repo string) bool {
	for _, l := range r.Capability.Identity.Labels {
		if v, ok := strings.CutPrefix(l, repoLabel); ok {
			return strings.EqualFold(v, repo)
		}
	}
	return false
}
