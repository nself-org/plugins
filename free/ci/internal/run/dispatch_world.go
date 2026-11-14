package run

// Purpose: the placement inputs of the dispatch hook (P7-AUTO-28): the run's
// world, its effective scheduling policy and the scheduler job of an attempt.
// Inputs:  the run Session, the pipeline row and the attempt being placed.
// Outputs: sched.World + digest, sched.Policy, sched.Job.
// The policy carries the spend keys (economics hooks, econ_seam.go, D-1280).
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
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/trust"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

// projectID is the one project id the run places with, the id a node is authorized for: the forge
// repo name under serve (Session.ProviderRepo; Root is a temp clone there), else the repository's
// policy.ProjectIDGit (symlinks resolved, a linked worktree mapped to the main checkout). The pipeline
// row's Project stays the checkout root, which the cache, record and transport hooks read as a path.
func (rd *runDispatch) projectID(ctx context.Context) string {
	if rd.sess.ProviderRepo != "" {
		return rd.sess.ProviderRepo
	}
	return policy.ProjectIDGitCached(ctx, policy.Runner(report.Capture), rd.row.Project)
}

// schedJob maps the attempt with the mapping `ci plan` uses (planview.SchedJob), then sets what
// only a run knows: the attempt key and number, the source trust, the record flag and the local
// demand. The job's own labels, tools, isolation floor and accelerators are dropped: the local
// declaration carries no measured facts for them until P7-TRUST-08, and the exec-time isolation
// hook is the gate for them. A TEAM job with secrets is placed remote-only with its own requirements
// (nodeJob, D-1534): it never runs on this machine.
func (rd *runDispatch) schedJob(s AttemptSpec, id, row string, cpu, mem int) sched.Job {
	want := wantPlatform(s.Platform)
	c := planview.Context{PipelineID: rd.sess.RunID, Project: rd.projectID(context.Background()), Support: []string{platform()}, DefaultPlatform: platform(), Foreground: s.Foreground}
	if p := rd.sess.Plan; p != nil && len(p.Support) > 0 {
		c.Support = p.Support
	}
	if !slices.Contains(c.Support, want) { // an explicit platform outside the host-only default support is still the job's own ask
		c.Support = append(slices.Clone(c.Support), want)
	}
	job, node := nodeJob(rd.sess, *s.Job) // a TEAM job with secrets goes to a leased node in its node form (secrets_node.go)
	j := planview.SchedJob(c, planview.PlannedJob{ID: row, Name: row, Platform: s.Platform, Job: job})
	j.Key, j.Attempt, j.Trust = id, s.N, rd.jobTrust()
	j.User, j.Team = rd.sess.User, rd.sess.Team
	j.Pipeline.Band = attemptBand(s.Band, rd.row.Trigger)
	j.Accelerators = nil
	j.Pipeline.Record = rd.sess.Record
	req := model.PlacementRequirements{Platform: want, CPU: int64(cpu), MemMB: int64(mem)}
	if job.RemoteOnly || node { // the only jobs that go to a node with requirements: the world can check them against the node's facts
		req.Labels, req.Tools, req.IsolationMin, req.Network = j.Requirements.Labels, j.Requirements.Tools, j.Requirements.IsolationMin, j.Requirements.Network
		if job.Requirements != nil {
			j.Accelerators = job.Requirements.Accelerators
		}
	}
	j.Requirements = req
	j.RemoteOnly = j.RemoteOnly || node
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
		cp := *sp
		rd.spend = rd.sess.Spend
		rd.applySpend(&cp)
		rd.pol, rd.qp = &cp, *rd.sess.Queue
		return cp, nil
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
	if sp, serr := schedpolicy.Spend(&got); serr == nil { // a refused spend policy leaves the defaults: nothing paid
		rd.spend = &sp
	}
	rd.applySpend(&p)
	rd.pol, rd.qp = &p, qp
	return p, nil
}

// jobTrust is the class every job of the run is placed at: the class the run was admitted at, else the
// class the row records.
func (rd *runDispatch) jobTrust() model.TrustClass {
	if rd.sess.AdmitTrust != "" { // the row records the requested class; placement uses the class the run was admitted at
		return rd.sess.AdmitTrust
	}
	return model.TrustClass(rd.row.SourceTrust)
}

// world builds the placement world of the run: this machine as "local" (probe
// facts, the PERSONAL own node declaration, docker and tools as the run's plan
// measured them, else docker unknown) and every registered node.
func (rd *runDispatch) world(ctx context.Context, p sched.Pipeline) (sched.World, string, error) {
	capacity := rd.sess.Capacity()
	in := world.LocalInput{Project: rd.projectID(ctx), Platform: platform(), Capacity: capacity, DockerUnknown: true, Now: rd.sess.now()}
	if pl := rd.sess.Plan; pl != nil && pl.World.Measured { // the plan's measured facts, so `ci plan` and the run agree on this machine (D-1104)
		in.Docker, in.Tools, in.DockerUnknown = pl.World.Docker, pl.World.Tools, false
	}
	// what no job of this run can use is not produced (P7-CI-46): every job carries jobTrust, and a run
	// without a provider repo drops every hosted runner below (dropProviderRunners)
	hostedRepo := rd.hostedRepo(ctx)
	ctx = world.WithRunUse(ctx, world.RunUse{NoHosted: hostedRepo == "", NoNetworkProof: !trust.NetworkProofMatters(rd.jobTrust())})
	w, digest, err := world.BuildForLocal(ctx, world.Deps{Store: rd.sess.Store, JobIDs: rd.sess.jobRows(), Policy: rd.policy, Clock: rd.sess.now}, in, p)
	if err != nil {
		return sched.World{}, "", err
	}
	w, changed := dropProviderRunners(w, hostedRepo)
	if !changed {
		return w, digest, nil
	}
	digest, err = world.Digest(w)
	return w, digest, err
}

// hostedRepo is the repository whose hosted runners this run may use: the coordinator's
// Session.ProviderRepo, else for a bare run the github.com origin of the checkout (P7-WIRE-04), so
// prefer-included can place on GitHub-hosted without serve. It fails closed (D-1382, a private
// repository's baseline keeps hosted off): only a repository positively known public is bound; private,
// unknown, a non-github origin or no origin yields "" and NoHosted stays on.
func (rd *runDispatch) hostedRepo(ctx context.Context) string {
	if rd.sess.ProviderRepo != "" {
		return rd.sess.ProviderRepo
	}
	if pol, err := rd.policy(ctx, ""); err != nil || pol.Preset == "" || pol.Preset == "prefer-local" {
		return "" // a local-first policy never reaches for hosted: no origin read, no visibility call
	}
	if v, ok := bareRepos.Load(rd.row.Project); ok {
		return v.(string)
	}
	repo := bareHostedRepo(ctx, rd.row.Project)
	if ctx.Err() == nil {
		bareRepos.Store(rd.row.Project, repo)
	}
	return repo
}

// bareRepos keeps the answer per checkout root: a one-shot run builds its world on every dispatcher tick.
var bareRepos sync.Map

func bareHostedRepo(ctx context.Context, root string) string {
	if root == "" {
		return ""
	}
	capture := policy.Runner(report.Capture)
	url, ok := originURLFromConfig(root)
	if !ok { // a linked worktree or a bare layout: ask git
		out, exit, err := capture(ctx, "git", "-c", "core.fsmonitor=false", "-C", root, "config", "--get", "remote.origin.url")
		if err != nil || exit != 0 {
			return ""
		}
		url = out
	}
	owner, name, ok := report.ParseRemote(url)
	if !ok {
		return ""
	}
	if RepoVisibility != nil {
		if RepoVisibility(owner, name) != attest.Public {
			return ""
		}
		return owner + "/" + name
	}
	out, exit, err := capture(ctx, "gh", "api", "repos/"+owner+"/"+name, "--jq", ".visibility")
	if err != nil || exit != 0 || strings.TrimSpace(out) != "public" {
		return ""
	}
	return owner + "/" + name
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

// originURLFromConfig reads remote "origin"'s url from root/.git/config without spawning git (a run's
// spawn budget is tight). ok is false when root/.git/config cannot be read (a linked worktree); the caller then asks git, which also honours includes.
func originURLFromConfig(root string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(root, ".git", "config"))
	if err != nil {
		return "", true // the file was read and names no origin
	}
	in := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "["):
			in = strings.EqualFold(strings.Join(strings.Fields(line), " "), `[remote "origin"]`)
		case in:
			if k, v, found := strings.Cut(line, "="); found && strings.EqualFold(strings.TrimSpace(k), "url") {
				return strings.TrimSpace(v), true
			}
		}
	}
	return "", true // read, and it names no origin
}
