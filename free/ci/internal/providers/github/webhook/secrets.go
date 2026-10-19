package webhook

// Purpose: the operator key providers.github.webhook_secrets (map repository -> secret) and the
// binding of a repository name to its numeric id (Epic D8, review F2).
// Inputs:  the effective policy (UsePolicy installs the source; the coordinator start calls it with
//          the same function it gives runtime.UsePolicy); environment variables named by the key.
// Outputs: Keys (the key spec registered at init), SecretsFrom, UsePolicy, and the id pins.
// Constraints: operator scope only (Spend), Fixed, nil registered default (a Fixed key with a default
//          could never be set, see the HOST-16 ruling). A value is an environment variable NAME, never
//          a secret. A value is either "NAME" or {"env": "NAME", "id": 123}; with an id, a delivery
//          whose repository.id differs is refused. Without one the first verified id seen for a name
//          is pinned for the life of the process, so a repository deleted and recreated under the
//          same name cannot ride the old secret's trust.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/policy"
)

// KeyWebhookSecrets is the policy key HOST-05 owns.
const KeyWebhookSecrets = "providers.github.webhook_secrets"

// SharedSecretEnv is the legacy shared secret of the v1.4 `ci serve`.
const SharedSecretEnv = "GITHUB_WEBHOOK_SECRET"

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

// RepoSecret names where one repository's secret lives and, optionally, the repository id it is for.
type RepoSecret struct {
	Env string `json:"env"`
	ID  int64  `json:"id,omitempty"`
}

// Keys returns the spec of the key this package owns.
func Keys() []policy.KeySpec {
	return []policy.KeySpec{{Key: KeyWebhookSecrets, Owner: "P7-HOST-05", Kind: policy.TightenFixed, Spend: true, Decode: decodeSecrets}}
}

func decodeSecrets(raw json.RawMessage) (any, error) {
	var in map[string]json.RawMessage
	d := json.NewDecoder(bytes.NewReader(raw))
	if err := d.Decode(&in); err != nil || in == nil || d.More() {
		return nil, fmt.Errorf("E672 %s: expected an object of repository to environment variable name", KeyWebhookSecrets)
	}
	out := make(map[string]RepoSecret, len(in))
	for repo, v := range in {
		key := strings.ToLower(repo)
		if !coordinator.ValidProject(repo) {
			return nil, fmt.Errorf("E672 %s: %q is not an owner/repo name", KeyWebhookSecrets, repo)
		}
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("E672 %s: %q appears twice (names are case-insensitive)", KeyWebhookSecrets, repo)
		}
		var rs RepoSecret
		if err := json.Unmarshal(v, &rs.Env); err != nil { // not a plain name: the object form
			dd := json.NewDecoder(bytes.NewReader(v))
			dd.DisallowUnknownFields()
			if dd.Decode(&rs) != nil {
				return nil, fmt.Errorf("E672 %s: %q needs a variable name or {env, id}", KeyWebhookSecrets, repo)
			}
		}
		if !envName.MatchString(rs.Env) || rs.ID < 0 {
			return nil, fmt.Errorf("E672 %s: %q needs an environment variable name such as MY_REPO_SECRET and a positive id", KeyWebhookSecrets, repo)
		}
		out[key] = rs
	}
	return out, nil
}

// SecretsFrom reads the key from the effective policy; no value means no per-repo secrets. The
// result is keyed by lower-case repository name.
func SecretsFrom(eff *policy.Effective) (map[string]RepoSecret, error) {
	if eff == nil {
		return nil, fmt.Errorf("webhook: no effective policy")
	}
	v, _, ok := eff.Value(KeyWebhookSecrets)
	if !ok {
		return nil, nil
	}
	raw, err := json.Marshal(v) // the policy may hand back the decoded map or its generic JSON form
	if err != nil {
		return nil, err
	}
	got, err := decodeSecrets(raw)
	if err != nil {
		return nil, err
	}
	return got.(map[string]RepoSecret), nil
}

var policySource struct {
	sync.RWMutex
	fn func(context.Context) (*policy.Effective, error)
}

// UsePolicy installs the source of the operator's effective policy (the coordinator start does this
// once, next to runtime.UsePolicy). With none installed there are no per-repo secrets. nil removes it.
func UsePolicy(fn func(context.Context) (*policy.Effective, error)) {
	policySource.Lock()
	defer policySource.Unlock()
	policySource.fn = fn
}

func currentSecrets(ctx context.Context) (map[string]RepoSecret, error) {
	policySource.RLock()
	fn := policySource.fn
	policySource.RUnlock()
	if fn == nil {
		return nil, nil
	}
	eff, err := fn(ctx)
	if err != nil {
		return nil, err
	}
	return SecretsFrom(eff)
}

// pinTable remembers the first verified repository id of each name.
type pinTable struct {
	mu  sync.Mutex
	ids map[string]int64
}

// holds reports whether id is the id pinned for name, pinning it on first sight.
func (p *pinTable) holds(name string, id int64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ids == nil {
		p.ids = map[string]int64{}
	}
	if have, ok := p.ids[name]; ok {
		return have == id
	}
	p.ids[name] = id
	return true
}
