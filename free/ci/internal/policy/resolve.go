package policy

// Purpose: resolve the effective policy for one run (contract:ci.policy v1).
// Scopes, loosest first: defaults by visibility and source class, operator
// store rows (user, team, project), the protected default branch's `policy:`
// at the pinned tip, the revision's own `policy:` (tighten-only), and the
// per-run values (tighten-only, except the preset preference).
// Inputs: a Repo, a Revision, a key set, a store reader and a Runner.
// Outputs: Resolved: Effective, Values, Sources, PinnedCommit, Fetch, Digest
// and Ignored.
// Constraints: no write path; Spend keys never come from the branch or the
// revision (trust.Merge ignores them with E671); unknown keys anywhere are
// E672; with no origin only defaults and operator rows apply; the digest
// binds the pinned commit.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// LocalSubject is the user-scope subject of the single PERSONAL operator.
const LocalSubject = "local"

// Repo names what the policy is resolved for.
type Repo struct {
	Root       string           // repository root (git is run here)
	Project    string           // project-scope subject; see ProjectID
	User, Team string           // user and team subjects; User defaults to LocalSubject, an empty Team has no rows
	Visibility string           // public, private or unknown
	Class      model.TrustClass // source class that picks the default scope; empty counts as untrusted
}

// Revision is the checkout under test: Dir's `.nself/ci.yaml` `policy:` is the
// tighten-only revision scope. An empty Dir means no revision scope. Run holds
// the per-run values (the flags of one invocation); see runscope.go.
type Revision struct {
	Dir string
	Run map[string]json.RawMessage
}

// Reader is the store surface the resolver reads.
type Reader interface {
	PolicyRows(ctx context.Context, scope, subject string) ([]store.PolicyRow, error)
	PolicyPinFor(ctx context.Context, subject string) (store.PolicyPin, bool, error)
}

// Resolver loads and merges the policy scopes.
type Resolver struct {
	Keys  *Keyset // nil means Default
	Store Reader  // nil means no operator rows and no pin
	Run   Runner  // required: the exec funnel
}

// Resolved is the answer of Resolve.
type Resolved struct {
	Effective    Effective
	Values       map[string]any    // key -> effective value
	Sources      map[string]Source // key -> scope it came from
	PinnedCommit string
	Fetch        FetchState
	Digest       Digest
	Ignored      []Ignored
}

// Resolve merges every scope for repo and rev.
func (r *Resolver) Resolve(ctx context.Context, repo Repo, rev Revision) (Resolved, error) {
	if r.Run == nil {
		return Resolved{}, refuse("policy resolver has no process runner")
	}
	keys := r.Keys
	if keys == nil {
		keys = Default
	}
	reg, err := keys.registry()
	if err != nil {
		return Resolved{}, err
	}
	class := repo.Class
	scopes := []trust.Scope{trust.DefaultPolicy(repo.Visibility, class)}
	if class == "" {
		scopes = []trust.Scope{trust.DefaultPolicy(repo.Visibility)}
	}
	operator, err := r.operatorScopes(ctx, repo)
	if err != nil {
		return Resolved{}, err
	}
	scopes = append(scopes, operator.scopes...)

	var ignored []Ignored
	res := Resolved{Fetch: FetchNone}
	branch, err := readBranch(ctx, r.Run, repo.Root)
	if err != nil {
		return Resolved{}, err
	}
	res.Fetch = branch.Fetch
	if branch.Assumed {
		ignored = append(ignored, Ignored{Key: "default_branch", Scope: SourceProtectedBranch, Reason: "E672 origin's default branch could not be discovered; main is assumed"})
	}
	if branch.Name != "" && branch.Tip == "" {
		ignored = append(ignored, Ignored{Key: "protected_branch", Scope: SourceProtectedBranch, Reason: "E672 origin exists but origin/" + branch.Name + " could not be fetched or read; no protected-branch scope applies"})
	}
	var tipPolicy map[string]json.RawMessage
	if branch.Tip != "" {
		commit, note := chooseCommit(ctx, r.Run, repo.Root, branch, operator.pin)
		ignored = append(ignored, note...)
		if tipPolicy, err = policyAt(ctx, r.Run, repo.Root, commit); err != nil {
			return Resolved{}, err
		}
		res.PinnedCommit = commit
		scopes = append(scopes, trust.Scope{Kind: SourceProtectedBranch, Values: tipPolicy, PinnedCommit: commit})
	}
	if rev.Dir != "" {
		revPolicy, err := worktreePolicy(rev.Dir)
		if err != nil {
			return Resolved{}, err
		}
		if revPolicy = withoutSame(revPolicy, tipPolicy); len(revPolicy) > 0 {
			scopes = append(scopes, trust.Scope{Kind: SourceRevision, Values: revPolicy})
		}
	}
	if run := runScope(rev.Run); run != nil {
		scopes = append(scopes, *run)
	}
	eff, digest, err := reg.Merge(res.PinnedCommit, scopes...)
	if err != nil {
		return Resolved{}, err
	}
	res.Effective, res.Digest = eff, digest
	res.Ignored = append(ignored, eff.Ignored()...)
	res.Values, res.Sources = collect(eff, keys.Keys(), scopes)
	return res, nil
}

type operatorPolicy struct {
	scopes []trust.Scope
	pin    string
}

// operatorScopes reads the user, team and project rows and the stored pin.
func (r *Resolver) operatorScopes(ctx context.Context, repo Repo) (operatorPolicy, error) {
	var out operatorPolicy
	if r.Store == nil {
		return out, nil
	}
	user := repo.User
	if user == "" {
		user = LocalSubject
	}
	for _, s := range []struct {
		kind    Source
		scope   string
		subject string
	}{{SourceUser, "user", user}, {SourceTeam, "team", repo.Team}, {SourceProject, "project", repo.Project}} {
		if s.subject == "" {
			continue
		}
		rows, err := r.Store.PolicyRows(ctx, s.scope, s.subject)
		if err != nil {
			return operatorPolicy{}, fmt.Errorf("read %s policy: %w", s.scope, err)
		}
		if len(rows) == 0 {
			continue
		}
		values := make(map[string]json.RawMessage, len(rows))
		for _, row := range rows {
			values[row.Key] = row.Value
		}
		out.scopes = append(out.scopes, trust.Scope{Kind: s.kind, Values: values})
	}
	if repo.Project != "" {
		pin, ok, err := r.Store.PolicyPinFor(ctx, repo.Project)
		if err != nil {
			return operatorPolicy{}, fmt.Errorf("read policy pin: %w", err)
		}
		if ok {
			out.pin = pin.SHA
		}
	}
	return out, nil
}

// withoutSame drops revision keys whose value equals the protected branch's, so
// a worktree that matches main adds no scope of its own.
func withoutSame(revision, tip map[string]json.RawMessage) map[string]json.RawMessage {
	if len(revision) == 0 || len(tip) == 0 {
		return revision
	}
	out := map[string]json.RawMessage{}
	for k, v := range revision {
		if t, ok := tip[k]; ok && sameJSON(v, t) {
			continue
		}
		out[k] = v
	}
	return out
}

func sameJSON(a, b json.RawMessage) bool {
	var x, y bytes.Buffer
	if json.Compact(&x, a) != nil || json.Compact(&y, b) != nil {
		return false
	}
	return bytes.Equal(x.Bytes(), y.Bytes())
}

// collect lists the value and source of every key a registry default or a scope
// names. Effective has no key iterator, and a `prefix.*` family only has keys a
// scope gave.
func collect(eff Effective, specs []KeySpec, scopes []trust.Scope) (map[string]any, map[string]Source) {
	names := map[string]bool{}
	for _, s := range specs {
		if len(s.Key) < 2 || s.Key[len(s.Key)-2:] != ".*" {
			names[s.Key] = true
		}
	}
	for _, sc := range scopes {
		for k := range sc.Values {
			names[k] = true
		}
	}
	sorted := make([]string, 0, len(names))
	for k := range names {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	values, sources := map[string]any{}, map[string]Source{}
	for _, k := range sorted {
		if v, src, ok := eff.Value(k); ok {
			values[k], sources[k] = v, src
		}
	}
	return values, sources
}
