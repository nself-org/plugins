package tart

// Purpose: the Tart VM isolation profile for macOS (`vm` and `ephemeral-vm`, Epic D10).
// Inputs:  the operator's Tart settings (base image by digest, ssh user and key), the job spec from the
//          Isolator hook, and the network facts from netscope.
// Outputs: an isolation.Profile that probes Tart, and a JobSpec whose command is one ssh session into a
//          fresh clone; the clone is created in BeforeJob and destroyed in Release on every exit path.
// Constraints: Tart and softnet are installed by the user, never bundled or downloaded; no pf anchor, no
//          sudo, no host directory mount; secrets travel only as stdin script lines; scopes below lan need
//          softnet with block rules or the level is unavailable (E679); free VM slots are never oversubscribed.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Stable reasons next to E678/E679 (codes live in internal/isolation, Epic D25).
const (
	ReasonBaseImage = "isolation.tart_base_image"
	ReasonSlots     = "isolation.tart_no_free_slot"
	ReasonSecrets   = "isolation.tart_secrets_unreachable"
	// LicenceNote is shown by `ci doctor`: nSelf only runs the user's own install.
	LicenceNote = "Tart and softnet are third-party tools under their own licences; you install them, nSelf neither bundles nor redistributes them"
	// MaxSlots is Apple's limit of concurrent macOS VMs on one host.
	MaxSlots = 2
)

var baseImageRef = regexp.MustCompile(`^[A-Za-z0-9][^\s@]*@sha256:[0-9a-f]{64}$`)

// Options configures a Profile; every field has a default.
type Options struct {
	// Tart and SSH are the binaries ("tart", "/usr/bin/ssh").
	Tart, SSH string
	// BaseImage reads ci.tart.base_image from operator scope; the digest (@sha256:) is required.
	BaseImage func() string
	// SSHUser and SSHKey come from ci.tart.ssh_user (default "admin") and ci.tart.ssh_key (optional identity file).
	SSHUser, SSHKey func() string
	// Gateway is the host address of the Tart NAT network (default 192.168.64.1), the only address softnet allows.
	Gateway string
	// Home is the CI home ("" = NSELF_CI_HOME or ~/.nself/ci).
	Home func() string
	// NewTool builds the process starter for one coordinator; tests replace it.
	NewTool func(coordinator, home string) Tool
	// Net supplies the netscope facts and the proxy for restricted and internet.
	Net NetSource
	// GOOS and GOARCH override the platform (tests).
	GOOS, GOARCH string
	// Slots is the slot ledger; share one between the vm and ephemeral-vm profiles (default: a new one).
	Slots *Slots
	// TTL is how long a probe stands (default 20s).
	TTL time.Duration
	Now func() time.Time
	// Log receives operator-visible notes (leftovers that could not be removed).
	Log func(format string, args ...any)
}

// Profile is the Tart isolation profile for one level.
type Profile struct {
	level model.Isolation
	opts  Options

	mu      sync.Mutex
	av      isolation.Availability
	at      time.Time
	slots   *Slots
	sweepAt map[string]time.Time
	dirs    map[string]bool // state directories of attempts this process wrapped
}

var _ isolation.Profile = (*Profile)(nil)
var _ isolation.Lister = (*Profile)(nil)

// New returns the profile for level (vm or ephemeral-vm).
func New(level model.Isolation, o Options) *Profile {
	if o.Tart == "" {
		o.Tart = "tart"
	}
	if o.SSH == "" {
		o.SSH = "/usr/bin/ssh" // SIP-protected: dyld ignores DYLD_* for it
	}
	if o.GOOS == "" {
		o.GOOS = runtime.GOOS
	}
	if o.GOARCH == "" {
		o.GOARCH = runtime.GOARCH
	}
	if o.Gateway == "" {
		o.Gateway = "192.168.64.1"
	}
	if o.TTL <= 0 {
		o.TTL = 20 * time.Second
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Log == nil {
		o.Log = func(format string, args ...any) { fmt.Fprintf(os.Stderr, "tart: "+format+"\n", args...) }
	}
	if o.NewTool == nil {
		o.NewTool = NewExecTool
	}
	if o.Slots == nil {
		o.Slots = NewSlots()
	}
	return &Profile{slots: o.Slots, level: level, opts: o, sweepAt: map[string]time.Time{}, dirs: map[string]bool{}}
}

// Level is the declared level.
func (p *Profile) Level() model.Isolation { return p.level }

func (p *Profile) home(spec string) string {
	if spec != "" {
		return spec
	}
	if p.opts.Home != nil {
		if h := p.opts.Home(); h != "" {
			return h
		}
	}
	if h := os.Getenv("NSELF_CI_HOME"); h != "" {
		return h
	}
	if u, err := os.UserHomeDir(); err == nil {
		return filepath.Join(u, ".nself", "ci")
	}
	return filepath.Join(os.TempDir(), "nself-ci")
}

func (p *Profile) str(f func() string, dflt string) string {
	if f != nil {
		if v := strings.TrimSpace(f()); v != "" {
			return v
		}
	}
	return dflt
}

// Prepare checks, before the job starts, that Tart still works, and sweeps orphan clones of dead owners.
func (p *Profile) Prepare(ctx context.Context, a isolation.Attempt) error {
	av := p.probe(ctx, true)
	if !av.Available() {
		return isolation.Unavailable(av.Reason, "%s isolation is unavailable for attempt %s: %s", p.level, a.ID, av.Facts["hint"])
	}
	p.sweepOnce(ctx, a.CoordinatorID)
	return nil
}

// Teardown removes the clone of an attempt if it is still there; it is idempotent.
func (p *Profile) Teardown(ctx context.Context, a isolation.Attempt) error {
	tool := p.opts.NewTool(a.CoordinatorID, p.home(""))
	return destroyClone(ctx, tool, p.opts.Tart, CloneName(a.ID), nil)
}

// List returns the nself-ci clones Tart knows (SCHED-09 lists leftovers by label).
func (p *Profile) List(ctx context.Context, _ string) ([]string, error) {
	vms, err := listVMs(ctx, p.opts.NewTool("tart-list", p.home("")), p.opts.Tart)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, v := range vms {
		if strings.HasPrefix(v.Name, ClonePrefix) {
			out = append(out, v.Name)
		}
	}
	return out, nil
}
