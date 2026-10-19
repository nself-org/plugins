package gvisor

// Purpose: the sandboxed-container profile: the hardened container profile with runsc as runtime.
// Inputs:  an exec.JobSpec the Isolator hook hands to Wrap.
// Outputs: a spec whose planner adds --runtime=runsc to the container profile's own command.
// Constraints: all hardening (caps, user, limits, labels, network) stays in the container
// profile and exec's argv backstop still checks the final command, so this profile only adds the
// runtime; it never builds a container command of its own. Labels are the container profile's,
// so its List already finds these containers and this profile does not list (no duplicates).

import (
	"context"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/isolation/container"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Options configures a Profile; every field has a default.
type Options struct {
	Runner container.Runner
	// Separated reports whether this node is a separated node (agent user without docker group).
	Separated func() bool
	Network   isolation.NetworkPlanner
	UIDs      func() []int
	Starter   Starter
	// HasKVM reports whether /dev/kvm exists; Image is the probe image (default ProbeImage).
	HasKVM func() bool
	Image  string
	TTL    time.Duration
	Now    func() time.Time
}

// Profile is the sandboxed-container isolation profile.
type Profile struct {
	opts Options
	base *container.Profile
	mu   sync.Mutex
	av   isolation.Availability
	at   time.Time
}

var _ isolation.Profile = (*Profile)(nil)

// New returns a Profile with defaults filled in.
func New(o Options) *Profile {
	if o.Runner == nil {
		o.Runner = execRunner{}
	}
	if o.Starter == nil {
		o.Starter = execStarter{}
	}
	if o.HasKVM == nil {
		o.HasKVM = hasKVM
	}
	if o.Image == "" {
		o.Image = ProbeImage
	}
	if o.TTL <= 0 {
		o.TTL = time.Minute // a start costs more than the container profile's info query
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	base := container.New(container.Options{Runner: o.Runner, Separated: o.Separated, Network: o.Network, UIDs: o.UIDs})
	return &Profile{opts: o, base: base}
}

type execRunner struct{}

func (execRunner) Query(ctx context.Context, binary string, args ...string) ([]byte, error) {
	return exec.RuntimeQuery(ctx, binary, args...)
}

// Level declares the sandboxed-container level.
func (p *Profile) Level() model.Isolation { return "sandboxed-container" }

// Probe reports whether gVisor can run a job now, cached for the TTL.
func (p *Profile) Probe(ctx context.Context) isolation.Availability {
	p.mu.Lock()
	if !p.at.IsZero() && p.opts.Now().Sub(p.at) < p.opts.TTL {
		av := p.av
		p.mu.Unlock()
		return av
	}
	p.mu.Unlock()
	// asked outside the lock: the start is time boxed, and a wedged daemon must not block other callers
	av := p.probeFresh(ctx)
	p.mu.Lock()
	p.av, p.at = av, p.opts.Now()
	p.mu.Unlock()
	return av
}

// Prepare fails with E678 when gVisor stopped being usable.
func (p *Profile) Prepare(ctx context.Context, a isolation.Attempt) error {
	if av := p.Probe(ctx); !av.Available() {
		return isolation.Unavailable(av.Reason, "sandboxed-container isolation is unavailable for attempt %s", a.ID)
	}
	return nil
}

// Wrap lets the container profile plan the hardened command, then adds the runsc runtime to it.
func (p *Profile) Wrap(s exec.JobSpec) (exec.JobSpec, error) {
	if av := p.Probe(context.Background()); !av.Available() {
		return s, isolation.Unavailable(av.Reason, "sandboxed-container isolation is unavailable")
	}
	wrapped, err := p.base.Wrap(s)
	if err != nil {
		return s, err
	}
	wrapped.Plan = &planner{inner: wrapped.Plan}
	return wrapped, nil
}

// Teardown removes the attempt's container; the name and labels are the container profile's.
func (p *Profile) Teardown(ctx context.Context, a isolation.Attempt) error {
	return p.base.Teardown(ctx, a)
}

// planner implements exec.ContainerPlanner over the container profile's planner.
type planner struct{ inner exec.ContainerPlanner }

func (pl *planner) Plan(r exec.ContainerRun) (exec.ContainerCommand, error) {
	cmd, err := pl.inner.Plan(r)
	if err != nil {
		return cmd, err
	}
	return withRuntime(cmd)
}

func (pl *planner) Cleanup(r exec.ContainerRun) (*exec.ContainerCommand, error) {
	cmd, err := pl.inner.Cleanup(r)
	if err != nil || cmd == nil {
		return cmd, err
	}
	out, err := withRuntime(*cmd) // the cleanup helper runs the job UID's files too: same sandbox
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// withRuntime inserts --runtime=runsc right after `run`. The container profile never names a
// runtime, and exec's argv backstop admits no other value than runsc.
func withRuntime(cmd exec.ContainerCommand) (exec.ContainerCommand, error) {
	if len(cmd.Args) == 0 || cmd.Args[0] != "run" {
		return cmd, isolation.Unavailable(isolation.ReasonProbeFailed, "container profile planned a command that is not a run")
	}
	args := append([]string{"run", "--runtime=" + RuntimeName}, cmd.Args[1:]...)
	return exec.ContainerCommand{Binary: cmd.Binary, Args: args}, nil
}
