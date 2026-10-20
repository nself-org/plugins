package webhook

// Purpose: the two operator keys providers.gitlab.webhook_signing_secrets and
// providers.gitlab.webhook_tokens (map project -> environment variable name) and the binding of a
// project name to its numeric id (Epic D11, review F2).
// Inputs:  the effective policy (UsePolicy installs the source; the coordinator start calls it with the
//          same function it gives runtime.UsePolicy); environment variables named by the keys.
// Outputs: Keys (the specs registered at init), SecretsFrom, UsePolicy, and the id pins.
// Constraints: operator scope only (Spend), Fixed, nil registered default (a Fixed key with a default
//          could never be set, see the HOST-16 ruling). A value is an environment variable NAME, never
//          a secret. A value is either "NAME" or {"env": "NAME", "id": 123}; with an id, a delivery
//          whose project.id differs is refused. Without one the first verified id seen for a name is
//          pinned for the life of the process, so a project deleted and recreated under the same name
//          cannot ride the old secret's trust. The two keys use separate variable names; a project
//          listed under both is a signing project (the bare token is then never accepted). A project
//          name must be group/project: the coordinator accepts no deeper path.

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

// The policy keys HOST-21 owns.
const (
	KeySigningSecrets = "providers.gitlab.webhook_signing_secrets"
	KeyTokens         = "providers.gitlab.webhook_tokens"
)

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

// RepoSecret names where one project's secret lives and, optionally, the project id it is for.
type RepoSecret struct {
	Env string `json:"env"`
	ID  int64  `json:"id,omitempty"`
}

// Secrets is the operator's webhook policy, keyed by lower-case project name.
type Secrets struct {
	Signing map[string]RepoSecret
	Tokens  map[string]RepoSecret
}

// Keys returns the specs of the keys this package owns.
func Keys() []policy.KeySpec {
	return []policy.KeySpec{
		{Key: KeySigningSecrets, Owner: "P7-HOST-21", Kind: policy.TightenFixed, Spend: true, Decode: decoder(KeySigningSecrets)},
		{Key: KeyTokens, Owner: "P7-HOST-21", Kind: policy.TightenFixed, Spend: true, Decode: decoder(KeyTokens)},
	}
}

func decoder(key string) func(json.RawMessage) (any, error) {
	return func(raw json.RawMessage) (any, error) { return decodeSecrets(key, raw) }
}

func decodeSecrets(key string, raw json.RawMessage) (map[string]RepoSecret, error) {
	var in map[string]json.RawMessage
	d := json.NewDecoder(bytes.NewReader(raw))
	if err := d.Decode(&in); err != nil || in == nil || d.More() {
		return nil, fmt.Errorf("E672 %s: expected an object of project to environment variable name", key)
	}
	out := make(map[string]RepoSecret, len(in))
	for project, v := range in {
		lower := strings.ToLower(project)
		if !validName(project) {
			return nil, fmt.Errorf("E672 %s: %q is not a group/project name", key, project)
		}
		if _, dup := out[lower]; dup {
			return nil, fmt.Errorf("E672 %s: %q appears twice (names are case-insensitive)", key, project)
		}
		var rs RepoSecret
		if err := json.Unmarshal(v, &rs.Env); err != nil { // not a plain name: the object form
			dd := json.NewDecoder(bytes.NewReader(v))
			dd.DisallowUnknownFields()
			if dd.Decode(&rs) != nil {
				return nil, fmt.Errorf("E672 %s: %q needs a variable name or {env, id}", key, project)
			}
		}
		if !envName.MatchString(rs.Env) || rs.ID < 0 {
			return nil, fmt.Errorf("E672 %s: %q needs an environment variable name such as MY_PROJECT_SECRET and a non-negative id", key, project)
		}
		out[lower] = rs
	}
	return out, nil
}

// validName is a project the coordinator can enqueue: group/project, letters, digits, '.', '_', '-'.
func validName(name string) bool { return len(name) <= 255 && coordinator.ValidProject(name) }

// SecretsFrom reads both keys from the effective policy; no value means none.
func SecretsFrom(eff *policy.Effective) (Secrets, error) {
	if eff == nil {
		return Secrets{}, fmt.Errorf("webhook: no effective policy")
	}
	var s Secrets
	for _, k := range []struct {
		key  string
		into *map[string]RepoSecret
	}{{KeySigningSecrets, &s.Signing}, {KeyTokens, &s.Tokens}} {
		v, _, ok := eff.Value(k.key)
		if !ok {
			continue
		}
		raw, err := json.Marshal(v) // the policy may hand back the decoded map or its generic JSON form
		if err != nil {
			return Secrets{}, err
		}
		if *k.into, err = decodeSecrets(k.key, raw); err != nil {
			return Secrets{}, err
		}
	}
	return s, nil
}

var policySource struct {
	sync.RWMutex
	fn func(context.Context) (*policy.Effective, error)
}

// UsePolicy installs the source of the operator's effective policy (the coordinator start does this
// once, next to runtime.UsePolicy). With none installed there are no secrets. nil removes it.
func UsePolicy(fn func(context.Context) (*policy.Effective, error)) {
	policySource.Lock()
	defer policySource.Unlock()
	policySource.fn = fn
}

func currentSecrets(ctx context.Context) (Secrets, error) {
	policySource.RLock()
	fn := policySource.fn
	policySource.RUnlock()
	if fn == nil {
		return Secrets{}, nil
	}
	eff, err := fn(ctx)
	if err != nil {
		return Secrets{}, err
	}
	return SecretsFrom(eff)
}

// pinTable remembers the first verified project id of each name.
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
