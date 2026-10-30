package container

// Purpose: the hardened container profile: probe, wrap, teardown and list.
// Inputs:  an exec.JobSpec the Isolator hook hands to Wrap.
// Outputs: a spec carrying a ContainerPlanner; exec builds nothing itself for it.
// Constraints: no runtime means E678, never a process fallback; the probe result is
// cached briefly so a burst of jobs asks the daemon once.

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Options configures a Profile; every field has a default.
type Options struct {
	Runner Runner
	// Separated reports whether this node is a separated node (agent user without docker group).
	Separated func() bool
	Network   isolation.NetworkPlanner
	// UIDs are the host UIDs a job must never use; default the current process's.
	UIDs func() []int
	TTL  time.Duration
	Now  func() time.Time
}

// Profile is the container isolation profile.
type Profile struct {
	opts  Options
	mu    sync.Mutex
	facts Facts
	av    isolation.Availability
	at    time.Time
}

var _ isolation.Profile = (*Profile)(nil)
var _ isolation.Lister = (*Profile)(nil)

// New returns a Profile with defaults filled in.
func New(o Options) *Profile {
	if o.Runner == nil {
		o.Runner = execRunner{}
	}
	if o.Separated == nil {
		o.Separated = func() bool { return false }
	}
	if o.Network == nil {
		o.Network = isolation.DefaultNetworkPlanner()
	}
	if o.UIDs == nil {
		o.UIDs = func() []int { return []int{os.Getuid(), os.Geteuid()} }
	}
	if o.TTL <= 0 {
		o.TTL = 30 * time.Second
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Profile{opts: o}
}

// Level declares the container level.
func (p *Profile) Level() model.Isolation { return "container" }

// Probe reports the runtime this node can use, cached for the TTL.
func (p *Profile) Probe(ctx context.Context) isolation.Availability {
	_, av := p.probe(ctx)
	return av
}

func (p *Profile) probe(ctx context.Context) (Facts, isolation.Availability) {
	p.mu.Lock()
	if !p.at.IsZero() && p.opts.Now().Sub(p.at) < p.opts.TTL {
		facts, av := p.facts, p.av
		p.mu.Unlock()
		return facts, av
	}
	p.mu.Unlock()
	// the runtime is asked outside the lock (each query is time boxed), so a wedged daemon never blocks other callers
	facts, av := probeRuntime(ctx, p.opts.Runner, p.opts.Separated())
	p.mu.Lock()
	if ctx.Err() == nil {
		p.facts, p.av, p.at = facts, av, p.opts.Now()
	}
	p.mu.Unlock()
	return facts, av
}

// Prepare fails with E678 when the runtime stopped being usable.
func (p *Profile) Prepare(ctx context.Context, a isolation.Attempt) error {
	if _, av := p.probe(ctx); !av.Available() {
		return isolation.Unavailable(av.Reason, "container isolation is unavailable for attempt %s", a.ID)
	}
	return nil
}

// networkSetupTimeout bounds the creation of a per-attempt network and its proxy sidecar inside Wrap.
const networkSetupTimeout = 4 * time.Minute

// Wrap attaches the planner; the network scope is planned now so E679 surfaces before any workspace exists.
// A scope that needs per-attempt resources (restricted, internet) creates them here and hands their removal
// to the spec's Release, which the executor runs on every path.
func (p *Profile) Wrap(s exec.JobSpec) (exec.JobSpec, error) {
	facts, av := p.probe(context.Background())
	if !av.Available() {
		return s, isolation.Unavailable(av.Reason, "container isolation is unavailable")
	}
	pl := &planner{facts: facts, uids: p.opts.UIDs()}
	if _, err := SlotUID(s.Slot, pl.uids, pl.params(exec.ContainerRun{}).MaxUID); err != nil {
		return s, err // refused before any workspace exists
	}
	plan, err := p.planNetwork(&s, facts.Binary)
	if err != nil {
		return s, err
	}
	pl.net = plan
	s.Plan = pl
	return s, nil
}

// planNetwork plans the scope; for an isolation.AttemptPlanner it also creates the attempt's resources and
// chains their removal onto s.Release and the proxy variables onto s.NetEnv.
func (p *Profile) planNetwork(s *exec.JobSpec, binary string) (isolation.NetworkPlan, error) {
	ap, ok := p.opts.Network.(isolation.AttemptPlanner)
	if !ok {
		return p.opts.Network.Plan(s.Network)
	}
	ctx, cancel := context.WithTimeout(context.Background(), networkSetupTimeout)
	defer cancel()
	a := isolation.Attempt{ID: s.AttemptID, CoordinatorID: s.CoordinatorID, Slot: s.Slot}
	plan, err := ap.PlanAttempt(ctx, binary, a, s.Network)
	if err != nil {
		if plan.Release != nil {
			plan.Release(context.WithoutCancel(ctx))
		}
		return plan, err
	}
	if rel := plan.Release; rel != nil {
		prev := s.Release
		s.Release = func(ctx context.Context) {
			rel(ctx)
			if prev != nil {
				prev(ctx)
			}
		}
	}
	s.NetEnv = append(s.NetEnv, plan.Env...)
	return plan, nil
}

// Teardown removes the attempt's job, helper, proxy and network, including after a coordinator dies.
func (p *Profile) Teardown(ctx context.Context, a isolation.Attempt) error {
	facts, av := p.probe(ctx)
	if !av.Available() {
		return isolation.Unavailable(av.Reason, "cannot remove the container of attempt %s", a.ID)
	}
	return exec.RuntimeRemoveAttempt(ctx, facts.Binary, a.ID)
}

// List returns the names of containers (running or not) carrying label, for SCHED-09.
func (p *Profile) List(ctx context.Context, label string) ([]string, error) {
	facts, av := p.probe(ctx)
	if !av.Available() {
		// not "no leftovers": the sweep must know it could not look
		return nil, isolation.Unavailable(av.Reason, "cannot list labelled containers")
	}
	out, err := p.opts.Runner.Query(ctx, facts.Binary, "ps", "-a", "--filter", "label="+label, "--format", "{{.Names}}")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

// planner implements exec.ContainerPlanner over Build.
type planner struct {
	facts Facts
	uids  []int
	net   isolation.NetworkPlan
}

func (pl *planner) params(r exec.ContainerRun) Params {
	max := 0
	if pl.facts.Rootless {
		max = MaxRootlessUID
	}
	return Params{Run: r, Binary: pl.facts.Binary, ForbiddenUIDs: pl.uids, Network: pl.net, MaxUID: max,
		UsernsAuto: pl.facts.Binary == "podman" && !pl.facts.Rootless}
}

func (pl *planner) Plan(r exec.ContainerRun) (exec.ContainerCommand, error) {
	return Build(pl.params(r))
}

// cleanupScript removes whatever the job UID created under the workspace mount. It runs in CleanupImage, not the
// job's image, so it works when the job image has no shell.
const cleanupScript = `chmod -R u+rwX /repo 2>/dev/null; rm -rf -- /repo/* /repo/.[!.]* /repo/..?* 2>/dev/null; true`

func (pl *planner) Cleanup(r exec.ContainerRun) (*exec.ContainerCommand, error) {
	r.Name += "-clean"
	if !r.JobImageCleanup {
		r.Image = CleanupImage
	}
	r.Command = []string{"-c", cleanupScript}
	r.EnvFile, r.Workdir, r.CPU, r.MemoryMB, r.Mounts = "", "/repo", 0.5, 128, nil
	p := pl.params(r)
	p.Network, p.Entrypoint = isolation.NetworkPlan{Network: "none"}, "sh"
	cmd, err := Build(p)
	if err != nil {
		return nil, err
	}
	return &cmd, nil
}
