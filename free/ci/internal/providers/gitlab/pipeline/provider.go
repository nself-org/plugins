// Package pipeline is the GitLab pipeline dispatch provider (P7-HOST-06, Epic D11): it runs one
// attempt as a pipeline created through the pipeline API with the generated include file
// templates/gitlab, follows the pipeline through the provider REST funnel and accepts its evidence
// only through internal/providers/hostedev. It holds no trigger token and never calls the trigger
// endpoint.
package pipeline

// Purpose: the provider object: its configuration, the credential rule of the "gitlab"
//          provider id, the REST client it builds per call and the lease profile of a hosted run.
// Inputs:  Config (every field is optional; the zero Config is the production wiring).
// Outputs: *Provider, which implements providers.Provider and providers.Dispatcher; the process-wide
//          instance registered from register.go and dormant unless compat.V15().
// Constraints: the id is "gitlab", the only GitLab value the runner-capability schema allows for
//          identity.provider, and the credential rule of that id is the operator-policy one built into
//          runtime.Credential; the registry holds one provider per id, so the GitLab trigger source
//          (P7-HOST-21) plugs its TrustFacts into this provider through UseTrustFacts instead of
//          registering a second "gitlab"; a client is built per call from the operator's policy;
//          nothing here holds a token; Wait and Clock are injected by tests, so no unit test sleeps or
//          reads the wall clock.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/api"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

// ProviderID is the id the provider registers under and the identity.provider of its runners.
const ProviderID = "gitlab"

// DefaultUntrustedRef is the branch untrusted work is dispatched on. The project keeps it unprotected
// and without access to protected variables; the operator creates it (providers gitlab init --check
// says whether it exists in local git).
const DefaultUntrustedRef = "nself-ci/untrusted"

// MinProducerVersion is the lowest evidence producer version accepted by default (see hosted).
const MinProducerVersion = "1"

// Config tunes a Provider.
type Config struct {
	// ID overrides the provider id (default ProviderID); a test registers a second instance with it.
	ID string
	// Client builds the REST client of one call; nil builds it from the operator's policy.
	Client func(context.Context) (*rest.Client, error)
	Clock  func() time.Time
	// Wait pauses between two looks for a pipeline; nil sleeps.
	Wait func(context.Context, time.Duration) error
	// Nonce mints a correlation nonce; nil reads 128 random bits.
	Nonce   func() (string, error)
	Journal Journal
	// UntrustedRef is the branch for untrusted work; empty means DefaultUntrustedRef.
	UntrustedRef string
	// Rendering returns the coordinator's rendering of the include file a pipeline must have used; nil
	// uses the pinned renderings of this build (DefaultRendering).
	Rendering func(ctx context.Context, version int, file []byte) ([]byte, error)
	// Download fetches a link (artifact archive, raw file); nil uses the funnel's allowlisted Download.
	// Tests that cannot reach a real redirect host supply their own.
	Download func(ctx context.Context, c *rest.Client, link string, max int64, w io.Writer) error
	// MinAgent is the lowest producer version accepted from the evidence.
	MinAgent string
	// FindWindow is how long a pipeline is looked for after a dispatch (default one minute).
	FindWindow time.Duration
}

const (
	findStep     = 5 * time.Second
	findWindow   = time.Minute
	window       = 2 * time.Minute
	leaseTTL     = 30 * time.Minute
	leasePoll    = 20 * time.Second
	hostCapacity = 4
	userAgent    = "nself-ci-gitlab-pipeline"
)

// Provider is the GitLab pipeline dispatch provider.
type Provider struct {
	cfg     Config
	mu      sync.Mutex
	journal Journal
	login   string
	loginAt time.Time
	trust   providers.TrustFacts
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
	if cfg.UntrustedRef == "" {
		cfg.UntrustedRef = DefaultUntrustedRef
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
		return "", api.Unavailable("nonce")
	}
	return hex.EncodeToString(b[:]), nil
}

// ID is "gitlab".
func (p *Provider) ID() string { return p.cfg.ID }

// Profile is the lease profile: a hosted run renews from Liveness every 20 s and expires after 30 min.
func (*Provider) Profile() providers.LeaseProfile {
	return providers.LeaseProfile{TTL: leaseTTL, Poll: leasePoll}
}

func (p *Provider) client(ctx context.Context) (*rest.Client, error) {
	if p.cfg.Client != nil {
		return p.cfg.Client(ctx)
	}
	return api.NewClient(ctx, userAgent)
}

// whoami returns the username of the token, cached for five minutes.
func (p *Provider) whoami(ctx context.Context, c *rest.Client) (string, error) {
	p.mu.Lock()
	login, at := p.login, p.loginAt
	p.mu.Unlock()
	if login != "" && p.cfg.Clock().Sub(at) < 5*time.Minute {
		return login, nil
	}
	var u api.User
	if _, err := c.Do(ctx, "GET", api.UserPath, nil, nil, &u); err != nil {
		return "", err
	}
	if u.Username == "" {
		return "", api.Misconfig("user.username")
	}
	p.mu.Lock()
	p.login, p.loginAt = u.Username, p.cfg.Clock()
	p.mu.Unlock()
	return u.Username, nil
}

func (p *Provider) jrnl() Journal {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.journal
}

// UseJournal replaces the dispatch journal.
func (p *Provider) UseJournal(j Journal) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.journal = j
}
