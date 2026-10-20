// Package webhook is the GitLab trigger source and trust-fact refetcher of the hosted providers
// (P7-HOST-21, Epic D11). It verifies a delivery, normalizes it into model.TriggerFacts and refetches
// the facts a payload cannot be trusted for. It starts no server: NODE-10's intake owns HTTP, dedup
// and the semantic key, and the runtime ingress adapter (HOST-16) hands requests here.
package webhook

// Purpose: providers.TriggerSource for GitLab: Match on X-Gitlab-Event; Verify authenticates the raw
// body and returns the facts. Authentication order, per project: the project's signing secret from
// providers.gitlab.webhook_signing_secrets (Standard Webhooks, auth per-repo; the bare token is then
// never consulted, so a stripped signature cannot downgrade the delivery), else, only for a project
// with no signing secret, the X-Gitlab-Token from providers.gitlab.webhook_tokens (auth bearer, which
// the trust classifier caps at untrusted), else refusal. There is no shared secret and no insecure mode.
// Inputs:  the request headers, the raw body (capped by the intake at 25 MiB), policy, environment.
// Outputs: model.TriggerFacts; E706 signature, token or timestamp for any authentication or identity
//          failure (403); a *Decline for a verified delivery that must not enqueue (200 accepted:false).
// Constraints: identity, secret lookup and the allowlist key on the project the body names
//          (project.path_with_namespace bound to project.id); the merge request source project is only
//          a fact. Authentication always comes before any event decision, so an unauthenticated sender
//          learns nothing about which events are served. Dormant unless the adapter routes to it
//          (compat.V15()).

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/pipeline"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

// Name is the source and provider id of facts this package produces.
const Name = "gitlab"

// Request header names.
const (
	hdrEvent     = "X-Gitlab-Event"
	hdrEventUUID = "X-Gitlab-Event-UUID"
	hdrToken     = "X-Gitlab-Token"
	hdrMsgID     = "webhook-id"
	hdrTimestamp = "webhook-timestamp"
	hdrSignature = "webhook-signature"
)

// Source is the GitLab trigger source. The zero value is not usable: call New.
type Source struct {
	Now     func() time.Time                            // clock
	Getenv  func(string) string                         // secret lookup, at call time
	Policy  func(context.Context) (Secrets, error)      // the operator's secrets (lower-case name -> entry)
	NewAPI  func(context.Context) (*rest.Client, error) // the REST client for API facts
	pins    pinTable
	regErrs []error
}

// New returns a Source wired to the process: real clock and environment, the installed policy and the
// REST funnel under the operator's GitLab settings.
func New() *Source {
	return &Source{Now: time.Now, Getenv: os.Getenv, Policy: currentSecrets, NewAPI: defaultAPI}
}

// Default is the source registered with the provider registry.
var Default = New()

// RegistrationErrors returns what init() could not register; empty when everything is in place.
func RegistrationErrors() []error { return append([]error(nil), Default.regErrs...) }

func init() {
	for _, k := range Keys() {
		if err := policy.RegisterKey(k); err != nil {
			Default.regErrs = append(Default.regErrs, err)
		}
	}
	if err := providers.RegisterSource(Default); err != nil {
		Default.regErrs = append(Default.regErrs, err)
	}
	// The registry holds one provider per id and "gitlab" is the dispatch provider (HOST-06): the
	// refetch of trigger facts plugs into it instead of registering a second one.
	pipeline.Default().UseTrustFacts(Default)
}

// Name is "gitlab".
func (*Source) Name() string { return Name }

// Match claims every request that carries X-Gitlab-Event.
func (*Source) Match(r *http.Request) bool { return r.Header.Get(hdrEvent) != "" }

// HasSecrets reports whether any project has a secret of its own: the coordinator then starts without
// --secret and leaves the allowlist to the secret for these deliveries (HOST-16 ruling).
func (s *Source) HasSecrets() bool {
	p, err := s.Policy(context.Background())
	return err == nil && (len(p.Signing) > 0 || len(p.Tokens) > 0)
}

// Verify authenticates r, then normalizes the event.
func (s *Source) Verify(ctx context.Context, r *http.Request, body []byte) (model.TriggerFacts, error) {
	name, id := probeBody(body).identity() // scalars only: nothing else is decoded before the delivery authenticates
	auth, secret, err := s.authenticate(ctx, r, body, name)
	if err != nil {
		return model.TriggerFacts{}, err
	}
	if !s.identityHolds(name, id, auth, secret) {
		return model.TriggerFacts{}, errSignature()
	}
	f, err := s.normalize(ctx, auth, r.Header.Get(hdrEvent), deliveryID(r), body)
	f.Repo = strings.ToLower(f.Repo) // the secret was found by the lower-case name; the project must not differ by case
	return f, err
}

// authenticate returns the auth class and the secret entry that verified the delivery.
func (s *Source) authenticate(ctx context.Context, r *http.Request, body []byte, name string) (model.TriggerAuth, RepoSecret, error) {
	pol, err := s.Policy(ctx)
	if err != nil || name == "" {
		return "", RepoSecret{}, errSignature() // an unreadable policy fails closed
	}
	key := strings.ToLower(name)
	if rs, ok := pol.Signing[key]; ok {
		// A project with a signing secret accepts nothing else: no token fallback (review F2).
		if err := verifySigned(s.Getenv(rs.Env), r.Header.Get(hdrMsgID), r.Header.Get(hdrTimestamp), r.Header.Get(hdrSignature), body, s.Now()); err != nil {
			return "", RepoSecret{}, err
		}
		return "per-repo", rs, nil
	}
	if rs, ok := pol.Tokens[key]; ok {
		if err := verifyToken(s.Getenv(rs.Env), r.Header.Get(hdrToken)); err != nil {
			return "", RepoSecret{}, err
		}
		return "bearer", rs, nil
	}
	return "", RepoSecret{}, errSignature()
}

// identityHolds binds the project name to its numeric id. A configured id is the whole rule (it never
// consults the pin, so changing it in policy takes effect at once). With a signing secret and no
// configured id, the first verified id seen for the name is pinned for the life of the process. A bare
// token delivery is never pinned: its holder could claim any id for a token-only project and lock out the
// project's real deliveries. A name the coordinator cannot enqueue, or no id, never holds.
func (s *Source) identityHolds(name string, id int64, auth model.TriggerAuth, rs RepoSecret) bool {
	if !validName(name) || id <= 0 {
		return false
	}
	switch {
	case rs.ID != 0:
		return rs.ID == id
	case auth == "per-repo":
		return s.pins.holds(strings.ToLower(name), id)
	}
	return true
}

// deliveryID is webhook-id, else X-Gitlab-Event-UUID.
func deliveryID(r *http.Request) string {
	for _, h := range []string{hdrMsgID, hdrEventUUID} {
		if v := r.Header.Get(h); v != "" && len(v) <= maxIDLen {
			return v
		}
	}
	return ""
}

var (
	_ providers.TriggerSource = (*Source)(nil)
	_ providers.TrustFacts    = (*Source)(nil)
)
