package cloudvm

// Purpose: the cloud VM ephemeral provider (Epic D13, TEAM only): burst.Provider over the
// `nself infra server` commands. This file holds the type, its configuration and Estimate;
// provision.go, teardown.go and enroll.go hold the lifecycle calls.
// Inputs:  Config: the coordinator id, the spend policy reader, the pairing service, the intent view,
//          the funnel Runner (default: internal/exec) and the lifetimes of the burst engine.
// Outputs: a burst.Provider with id "hetzner" (Config.ProviderID) and class cloud.
// Constraints: no live cloud call anywhere (every call is an infra child; tests use a fake nself);
// the token is read from the token file per call and lives only in that child's environment;
// the provider is offered only through burst.Engine, which refuses it in PERSONAL mode (burst.team_only)
// and without PaidAllowed (paid.disabled) before any call here; every call needs the caller's deadline;
// Estimate is local arithmetic over the declared rate (no child, no network).

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/pairing"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
)

// DefaultProviderID is the provider id of the Hetzner recipe.
const DefaultProviderID = "hetzner"

// DefaultCallTimeout bounds one infra child when the caller's context has no deadline of its own (the
// engine runs some calls under context.WithoutCancel): a hung child must never leave a machine without a teardown path.
const DefaultCallTimeout = 3 * time.Minute

// BootLatencyMs is the declared time from create to a joined node (server boot, cloud-init, agent download, enrollment).
const BootLatencyMs = int64(150_000)

// Config configures a Provider.
type Config struct {
	// CoordinatorID labels every server (coordinator_id) so the sweeper never touches another coordinator's.
	CoordinatorID string
	// ProviderID is the id the spend policy keys use (default "hetzner").
	ProviderID string
	// Spend reads the effective spend policy (operator scope) at call time; required.
	Spend func(context.Context) (schedpolicy.SpendPolicy, error)
	// Pairing makes and withdraws the enrollment offers; required.
	Pairing Pairing
	// Intents says whether a burst intent is live; required.
	Intents Intents
	// Lifetime carries MaxLifetime and Grace of the burst engine (zero fields take its defaults);
	// the node certificate lives MaxLifetime + Grace.
	Lifetime burst.Config
	// Home is the CI state directory (default NSELF_CI_HOME, else ~/.nself/ci); the funnel and the
	// 0700 user-data directory live under it.
	Home string
	// Nself is the nself binary (default: nself on PATH, resolved per call).
	Nself string
	// Runner runs the infra child (default: the exec funnel).
	Runner Runner
	// Registry, when set, lets AfterIssue complete the facts of an enrolled burst node (nodefacts.go).
	Registry *registry.Registry
	// CallTimeout bounds an infra child whose context has no deadline (default DefaultCallTimeout).
	CallTimeout time.Duration
	// Now is the clock (default time.Now).
	Now func() time.Time
}

// Provider is the cloud VM ephemeral provider.
type Provider struct {
	cfg    Config
	mu     sync.Mutex
	joiner Joiner
	probed map[string]bool
	offers map[string]string // intent token -> pairing code, for withdrawal
	byID   map[string]string // server id -> intent token
}

// New validates cfg and builds the provider.
func New(cfg Config) (*Provider, error) {
	if cfg.CoordinatorID == "" || cfg.Spend == nil || cfg.Pairing == nil || cfg.Intents == nil || len(cfg.CoordinatorID) > burst.MaxLabelValue {
		return nil, fail(ReasonRecipe, nil, "a coordinator id, a spend policy reader, a pairing service and an intent view are required")
	}
	if cfg.ProviderID == "" {
		cfg.ProviderID = DefaultProviderID
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Home == "" {
		cfg.Home = ciHome()
	}
	if cfg.Runner == nil {
		cfg.Runner = funnelRunner{home: cfg.Home, coordinator: cfg.CoordinatorID}
	}
	p := &Provider{cfg: cfg, probed: map[string]bool{}, offers: map[string]string{}, byID: map[string]string{}}
	p.purgeStale()
	return p, nil
}

// bound gives ctx a deadline when it has none.
func (p *Provider) bound(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	d := p.cfg.CallTimeout
	if d <= 0 {
		d = DefaultCallTimeout
	}
	return context.WithTimeout(ctx, d)
}

// userDataDir is the 0700 directory of the user-data files.
func (p *Provider) userDataDir() string { return filepath.Join(p.cfg.Home, "burst-userdata") }

// purgeStale removes user-data files left by a coordinator that died while an infra child ran: a file
// older than the offer's lifetime holds a code that can no longer be redeemed, so nothing needs it.
func (p *Provider) purgeStale() {
	if p.cfg.Home == "" {
		return
	}
	entries, err := os.ReadDir(p.userDataDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		if info, ierr := e.Info(); ierr == nil && strings.HasPrefix(e.Name(), "ud-") && p.cfg.Now().Sub(info.ModTime()) > pairing.OfferTTL {
			_ = os.Remove(filepath.Join(p.userDataDir(), e.Name()))
		}
	}
}

func ciHome() string {
	if h := os.Getenv("NSELF_CI_HOME"); h != "" {
		return h
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".nself", "ci")
	}
	return ""
}

// ID implements burst.Provider.
func (p *Provider) ID() string { return p.cfg.ProviderID }

// Class implements burst.Provider.
func (p *Provider) Class() string { return burst.ClassCloud }

// Estimate implements burst.Provider: the boot latency and the economics class; when the declared
// rate prices the job's timeout it also checks the quote is usable (a missing or unbounded rate is an
// error, which the engine reads as an unknown latency; the budget step refuses an unpriced class anyway).
func (p *Provider) Estimate(ctx context.Context, s burst.Spec) (burst.Estimate, error) {
	pol, err := p.cfg.Spend(ctx)
	if err != nil {
		return burst.Estimate{}, fail(ReasonEstimate, err, "the spend policy could not be read")
	}
	q := economics.QuoteJob(pol.Rates[p.cfg.ProviderID], s.TimeoutMs, pol.MaxPaidTimeoutMs)
	if q.Reason != "" {
		return burst.Estimate{LatencyMs: BootLatencyMs, Class: economics.ClassEphemeral}, fail(ReasonEstimate, nil, "no usable declared rate for %s: %s", p.cfg.ProviderID, q.Reason)
	}
	return burst.Estimate{LatencyMs: BootLatencyMs, Class: economics.ClassEphemeral}, nil
}

// lookPath finds an executable named name: a name with a slash is taken as it is, any other is
// searched in PATH (this package never imports os/exec: the funnel is the only place that starts processes).
func lookPath(name string) (string, bool) {
	ok := func(p string) bool {
		st, err := os.Stat(p)
		return err == nil && st.Mode().IsRegular() && st.Mode().Perm()&0o111 != 0
	}
	if strings.ContainsRune(name, filepath.Separator) {
		return name, ok(name)
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		if p := filepath.Join(dir, name); ok(p) {
			return p, true
		}
	}
	return "", false
}

// Quote prices a job of the given spec under the declared rate of this provider: whole billing
// increments (Hetzner bills by the hour), then the minimum charge and the extras. A missing rate, or a
// timeout that is zero or above economics.max_paid_timeout, has a Reason and no usable figure.
func (p *Provider) Quote(ctx context.Context, s burst.Spec) (economics.Quote, error) {
	pol, err := p.cfg.Spend(ctx)
	if err != nil {
		return economics.Quote{}, fail(ReasonEstimate, err, "the spend policy could not be read")
	}
	return economics.QuoteJob(pol.Rates[p.cfg.ProviderID], s.TimeoutMs, pol.MaxPaidTimeoutMs), nil
}

// nself resolves the nself binary to an absolute path.
func (p *Provider) nself() (string, error) {
	name := p.cfg.Nself
	if name == "" {
		name = "nself"
	}
	path, found := lookPath(name)
	if !found {
		return "", fail(ReasonNoNself, nil, "%q is not on the PATH", name)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fail(ReasonNoNself, nil, "%q cannot be resolved", name)
	}
	return abs, nil
}

var _ burst.Provider = (*Provider)(nil)
