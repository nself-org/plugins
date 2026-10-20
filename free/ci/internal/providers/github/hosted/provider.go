package hosted

// Purpose: the provider object: its configuration, its registration with the provider registry, the
//          credential rule of the "github-hosted" provider id, the REST client it builds per call and
//          the lease profile of a hosted run.
// Inputs:  Config (every field is optional; the zero Config is the production wiring).
// Outputs: *Provider, which implements providers.Provider and providers.Dispatcher; the process-wide
//          instance registered from init() and dormant unless compat.V15().
// Constraints: a client is built per call from the operator's policy (API base URL, allow-private-base
//          flag) and the "github" credential rule, so a policy change takes effect without a restart;
//          the artifact redirect hosts come from templates/github/redirect_hosts.yaml; nothing here
//          holds a token (the AuthFunc resolves it on demand); Wait and Clock are injected by tests,
//          so no unit test sleeps or reads the wall clock.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
	githubtmpl "github.com/nself-org/plugins/free/ci/templates/github"
)

// Config tunes a Provider.
type Config struct {
	// ID overrides the provider id (default ProviderID); a test registers a second instance with it.
	ID string
	// Client builds the REST client of one call; nil builds it from the operator's policy.
	Client func(context.Context) (*rest.Client, error)
	// Settings reads the keys of this package; nil uses CurrentSettings.
	Settings func(context.Context) (Settings, error)
	Clock    func() time.Time
	// Wait pauses between two looks for a run; nil sleeps.
	Wait func(context.Context, time.Duration) error
	// Nonce mints a correlation nonce; nil reads 128 random bits.
	Nonce   func() (string, error)
	Journal Journal
	// Rendering returns the coordinator's rendering of the workflow a run must have used; nil uses
	// the pinned renderings of this build (DefaultRendering).
	Rendering func(ctx context.Context, version int, workflow []byte) ([]byte, error)
	// Download fetches an artifact link; nil uses the funnel's allowlisted Download. Tests that cannot
	// reach a real redirect host supply their own.
	Download func(ctx context.Context, c *rest.Client, link string, max int64, w io.Writer) error
	// MinAgent is the lowest producer version accepted from the evidence.
	MinAgent string
	// FindWindow is how long a run is looked for after a dispatch (default one minute).
	FindWindow time.Duration
}

// MinProducerVersion is the lowest evidence producer version accepted by default. The agent's exec
// writes the evidence format version here; it moves to the agent release once exec reports it.
const MinProducerVersion = "1"

const (
	apiVersion   = "2022-11-28"
	findStep     = 5 * time.Second
	findWindow   = time.Minute
	leaseTTL     = 30 * time.Minute
	leasePoll    = 20 * time.Second
	hostCapacity = 4
)

// Provider is the GitHub-hosted dispatch provider.
type Provider struct {
	cfg     Config
	mu      sync.Mutex
	journal Journal
	login   string
	loginAt time.Time
}

var (
	_ providers.Provider   = (*Provider)(nil)
	_ providers.Dispatcher = (*Provider)(nil)
)

// New returns a Provider.
func New(cfg Config) *Provider {
	if cfg.ID == "" {
		cfg.ID = ProviderID
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.Wait == nil {
		cfg.Wait = func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		}
	}
	if cfg.Nonce == nil {
		cfg.Nonce = randomNonce
	}
	if cfg.MinAgent == "" {
		cfg.MinAgent = MinProducerVersion
	}
	if cfg.FindWindow <= 0 {
		cfg.FindWindow = findWindow
	}
	j := cfg.Journal
	if j == nil {
		j = NewMemJournal()
	}
	return &Provider{cfg: cfg, journal: j}
}

func randomNonce() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", failure(providers.Transient, "E704", "nonce")
	}
	return hex.EncodeToString(b[:]), nil
}

// ID is "github-hosted".
func (p *Provider) ID() string { return p.cfg.ID }

// Profile is the lease profile: a hosted run renews from Liveness every 20 s and expires after 30 min.
func (*Provider) Profile() providers.LeaseProfile {
	return providers.LeaseProfile{TTL: leaseTTL, Poll: leasePoll}
}

func (p *Provider) settings(ctx context.Context) (Settings, error) {
	if p.cfg.Settings != nil {
		return p.cfg.Settings(ctx)
	}
	return CurrentSettings(ctx)
}

func (p *Provider) client(ctx context.Context) (*rest.Client, error) {
	if p.cfg.Client != nil {
		return p.cfg.Client(ctx)
	}
	s, err := runtime.CurrentSettings(ctx)
	if err != nil {
		return nil, failure(providers.Config, "E700", "provider policy unreadable")
	}
	hosts, err := githubtmpl.RedirectHosts()
	if err != nil {
		return nil, misconfig("redirect_hosts")
	}
	return &rest.Client{BaseURL: s.GitHubAPIURL, Auth: runtime.Credential(ctx, "github"), RedirectHosts: hosts,
		UserAgent: "nself-ci-github-hosted", APIVersion: apiVersion, Provider: "github", AllowPrivateBase: s.GitHubAllowPrivateBase}, nil
}

// whoami returns the login of the token, cached for five minutes.
func (p *Provider) whoami(ctx context.Context, c *rest.Client) (string, error) {
	p.mu.Lock()
	login, at := p.login, p.loginAt
	p.mu.Unlock()
	if login != "" && p.cfg.Clock().Sub(at) < 5*time.Minute {
		return login, nil
	}
	var u actor
	if _, err := c.Do(ctx, "GET", userPath, nil, nil, &u); err != nil {
		return "", err
	}
	if u.Login == "" {
		return "", misconfig("user.login")
	}
	p.mu.Lock()
	p.login, p.loginAt = u.Login, p.cfg.Clock()
	p.mu.Unlock()
	return u.Login, nil
}
