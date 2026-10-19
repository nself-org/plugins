// Package webhook is the GitHub trigger source and trust-fact refetcher of the hosted providers
// (P7-HOST-05, Epic D8). It verifies a delivery, normalizes it into model.TriggerFacts and refetches
// the facts a payload cannot be trusted for. It starts no server: NODE-10's intake owns HTTP, dedup
// and the semantic key, and the runtime ingress adapter (HOST-16) hands requests here.
package webhook

// Purpose: providers.TriggerSource for GitHub: Match on X-GitHub-Event; Verify authenticates the raw
// body and returns the facts. Authentication order, per base repository: the repository's own secret
// from providers.github.webhook_secrets (auth per-repo; the shared secret is then never consulted),
// else the legacy GITHUB_WEBHOOK_SECRET (auth shared), else, only under the coordinator's --insecure,
// no check (auth none).
// Inputs:  the request headers, the raw body (capped by the intake at 25 MiB), policy, environment.
// Outputs: model.TriggerFacts; E706 "signature" for any authentication or identity failure (403); a
//          *Decline for a verified delivery that must not enqueue (200 accepted:false).
// Constraints: identity, secret lookup and the allowlist key on the BASE repository (repository.full_name
//          bound to repository.id); the PR head repository is only a fact. The signature is always
//          checked before any event decision, so an unauthenticated sender learns nothing about which
//          events are served. Dormant unless the adapter routes to it (compat.V15()).

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

// Name is the source and provider id of facts this package produces.
const Name = "github"

// Source is the GitHub trigger source. The zero value is not usable: call New.
type Source struct {
	Now      func() time.Time                                     // clock
	Getenv   func(string) string                                  // secret lookup, at call time
	Secrets  func(context.Context) (map[string]RepoSecret, error) // per-repo secrets (lower-case name -> secret)
	Insecure func() bool                                          // the coordinator's --insecure
	NewAPI   func(context.Context) (*rest.Client, error)          // the REST client for API facts
	pins     pinTable
}

// New returns a Source wired to the process: real clock and environment, the installed policy, the
// coordinator's --insecure flag and the REST funnel under the operator's GitHub settings.
func New() *Source {
	return &Source{Now: time.Now, Getenv: os.Getenv, Secrets: currentSecrets, Insecure: insecureFlag, NewAPI: defaultAPI}
}

var (
	flagMu       sync.RWMutex
	insecureFlg  *flag.Flag
	secretFlg    *flag.Flag
	registerErrs []error
)

// RegistrationErrors returns what init() could not register; empty when everything is in place.
func RegistrationErrors() []error { return append([]error(nil), registerErrs...) }

// Default is the source registered with the provider registry.
var Default = New()

func init() {
	for _, k := range Keys() {
		if err := policy.RegisterKey(k); err != nil {
			registerErrs = append(registerErrs, err)
		}
	}
	if err := providers.RegisterSource(Default); err != nil {
		registerErrs = append(registerErrs, err)
	}
	if err := providers.Register(Provider{Default}); err != nil {
		registerErrs = append(registerErrs, err)
	}
	coordinator.RegisterFlag(func(fs *flag.FlagSet) { // serve_v15 registers --insecure before AddFlags runs
		flagMu.Lock()
		insecureFlg, secretFlg = fs.Lookup("insecure"), fs.Lookup("secret")
		flagMu.Unlock()
	})
}

// insecureFlag reads the coordinator's --insecure; false until the flag set is known.
func insecureFlag() bool {
	flagMu.RLock()
	defer flagMu.RUnlock()
	return insecureFlg != nil && insecureFlg.Value.String() == "true"
}

// sharedSecret is the legacy shared secret: the coordinator's --secret (which overrides the
// environment in `ci serve`), else GITHUB_WEBHOOK_SECRET.
func (s *Source) sharedSecret() string {
	flagMu.RLock()
	var fromFlag string
	if secretFlg != nil {
		fromFlag = secretFlg.Value.String()
	}
	flagMu.RUnlock()
	if fromFlag != "" {
		return fromFlag
	}
	return s.Getenv(SharedSecretEnv)
}

// Name is "github".
func (*Source) Name() string { return Name }

// Match claims every request that carries X-GitHub-Event.
func (*Source) Match(r *http.Request) bool { return r.Header.Get(hdrEvent) != "" }

// HasSecrets reports whether any repository has its own secret: the coordinator then starts without
// --secret and leaves the allowlist to the secret for per-repo deliveries (HOST-16 ruling).
func (s *Source) HasSecrets() bool {
	m, err := s.Secrets(context.Background())
	return err == nil && len(m) > 0
}

// repoProbe is all that is read from a body before it is authenticated.
type repoProbe struct {
	Repository struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	} `json:"repository"`
}

// Verify authenticates r, then normalizes the event.
func (s *Source) Verify(ctx context.Context, r *http.Request, body []byte) (model.TriggerFacts, error) {
	var probe repoProbe
	_ = json.Unmarshal(body, &probe) // a body that does not parse names no repository and takes the shared or insecure path
	name := probe.Repository.FullName
	auth, secret, err := s.authenticate(ctx, r, body, name)
	if err != nil {
		return model.TriggerFacts{}, err
	}
	switch {
	case name == "" && r.Header.Get(hdrEvent) == "ping": // an organization hook's ping names no repository
		return model.TriggerFacts{}, unsupported("event \"ping\": skip")
	case auth == "none":
		if !coordinator.ValidProject(name) { // nothing authenticated the sender: still never pass junk on
			return model.TriggerFacts{}, unsupported("repository.full_name is not owner/repo")
		}
	case !s.identityHolds(name, probe.Repository.ID, secret):
		return model.TriggerFacts{}, errSignature()
	}
	return s.normalize(ctx, auth, r.Header.Get(hdrEvent), r.Header.Get(hdrDelivery), body)
}

// authenticate returns the auth class and the per-repo secret entry (zero for shared and none).
func (s *Source) authenticate(ctx context.Context, r *http.Request, body []byte, name string) (model.TriggerAuth, RepoSecret, error) {
	header := r.Header.Get(hdrSignature)
	secrets, err := s.Secrets(ctx)
	if err != nil {
		return "", RepoSecret{}, errSignature() // an unreadable policy fails closed
	}
	if rs, ok := secrets[strings.ToLower(name)]; ok && name != "" {
		if checkSignature(s.Getenv(rs.Env), header, body) != nil {
			return "", RepoSecret{}, errSignature() // no fallback to the shared secret (review F2)
		}
		return "per-repo", rs, nil
	}
	if shared := s.sharedSecret(); shared != "" {
		if checkSignature(shared, header, body) != nil {
			return "", RepoSecret{}, errSignature()
		}
		return "shared", RepoSecret{}, nil
	}
	if s.Insecure != nil && s.Insecure() {
		return "none", RepoSecret{}, nil
	}
	return "", RepoSecret{}, errSignature()
}

// identityHolds binds the repository name to its numeric id: the configured id when there is one, else
// the first verified id seen for the name. A name that is not owner/repo, or no id, never holds.
func (s *Source) identityHolds(name string, id int64, rs RepoSecret) bool {
	if !coordinator.ValidProject(name) || id <= 0 {
		return false
	}
	if rs.ID != 0 && rs.ID != id {
		return false
	}
	return s.pins.holds(strings.ToLower(name), id)
}

// Request header names.
const (
	hdrEvent     = "X-GitHub-Event"
	hdrDelivery  = "X-GitHub-Delivery"
	hdrSignature = "X-Hub-Signature-256"
)

var (
	_ providers.TriggerSource = (*Source)(nil)
	_ providers.TrustFacts    = (*Source)(nil)
)
