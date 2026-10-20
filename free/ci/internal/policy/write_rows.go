package policy

// Purpose: helpers of the policy write path (P7-TRUST-26): value checks, the
// stored-row reads, the merge check that keeps Resolve working, the loosening
// test and the machine-invoked refusal.
// Inputs: a Keyset, KeySpecs, JSON values and the Store.
// Outputs: compact values, merge errors (E670/E672) and E687 machine_invoked.
// Constraints: see write.go; no SQL here (the store owns it).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// checkValue decodes raw through the key's own Decode and returns it compact. A
// string outside the key's Order is refused (Decode alone would fail closed to
// the strictest value, which is not what the operator typed).
func checkValue(spec KeySpec, raw json.RawMessage) (json.RawMessage, error) {
	var buf bytes.Buffer
	if json.Compact(&buf, raw) != nil || buf.Len() == 0 {
		return nil, refuse("the value is not valid JSON")
	}
	compact := json.RawMessage(buf.Bytes())
	if _, err := spec.Decode(compact); err != nil {
		return nil, refuse("%s: %v", spec.Key, err)
	}
	var s string
	if len(spec.Order) > 0 && json.Unmarshal(compact, &s) == nil && !slices.Contains(spec.Order, s) {
		if spec.Secret {
			s = Masked
		}
		return nil, refuse("%s: %q is not one of %s", spec.Key, s, strings.Join(spec.Order, ", "))
	}
	return compact, nil
}

func shown(spec KeySpec, value json.RawMessage) string {
	if spec.Secret {
		return Masked
	}
	if len(value) > 200 {
		return string(value[:200]) + "..."
	}
	return string(value)
}

// rows reads the stored rows of the user, team and project subjects a resolve
// for c would merge, keyed by scope.
func (w *Writer) rows(ctx context.Context, t target, c Change) (map[string]map[string]json.RawMessage, error) {
	out := map[string]map[string]json.RawMessage{"user": {}, "team": {}, "project": {}}
	subjects := map[string]string{"user": LocalSubject, "team": c.Team, "project": c.Project}
	subjects[t.scope] = t.subject
	for scope, subject := range subjects {
		if subject == "" {
			continue
		}
		stored, err := w.Store.PolicyRows(ctx, scope, subject)
		if err != nil {
			return nil, fmt.Errorf("read %s policy: %w", scope, err)
		}
		for _, r := range stored {
			out[scope][r.Key] = r.Value
		}
	}
	return out, nil
}

// stored is the row a write replaces (nil when there is none).
func (w *Writer) stored(ctx context.Context, t target, key string) json.RawMessage {
	rows, err := w.Store.PolicyRows(ctx, t.scope, t.subject)
	if err != nil {
		return json.RawMessage("null") // unreadable: loosens against everything
	}
	for _, r := range rows {
		if r.Key == key {
			return r.Value
		}
	}
	return nil
}

// branchScope is the protected-branch scope a resolve of c's repository would
// merge (the pinned commit, else the fetched tip), or nil when there is none to
// read: no runner, no origin, an unreadable policy. The resolver fails with E670
// when that scope is looser than an operator row, so a write is checked with it.
func (w *Writer) branchScope(ctx context.Context, c Change) *trust.Scope {
	if w.Run == nil || c.Root == "" {
		return nil
	}
	b, err := readBranch(ctx, w.Run, c.Root)
	if err != nil || b.Tip == "" {
		return nil
	}
	pin := ""
	if c.Project != "" {
		if p, ok, err := w.Store.PolicyPinFor(ctx, c.Project); err == nil && ok {
			pin = p.SHA
		}
	}
	commit, _ := chooseCommit(ctx, w.Run, c.Root, b, pin)
	values, err := policyAt(ctx, w.Run, c.Root, commit)
	if err != nil {
		return nil
	}
	return &trust.Scope{Kind: SourceProtectedBranch, Values: values, PinnedCommit: commit}
}

// checkRows merges the operator rows and the protected-branch scope (when
// given) over the strictest default scopes the resolver can start from (an
// untrusted source on a private and on a public repository). It sees only the
// rows of the subjects it was given: another project's rows are not listed by
// the store, so they are not checked.
func (k *Keyset) checkRows(rows map[string]map[string]json.RawMessage, branch *trust.Scope) error {
	reg, err := k.registry()
	if err != nil {
		return err
	}
	kinds := []struct {
		scope string
		kind  trust.Source
	}{{"user", SourceUser}, {"team", SourceTeam}, {"project", SourceProject}}
	for _, vis := range []string{"private", "public"} {
		scopes := []trust.Scope{trust.DefaultPolicy(vis)}
		for _, kd := range kinds {
			if len(rows[kd.scope]) > 0 {
				scopes = append(scopes, trust.Scope{Kind: kd.kind, Values: rows[kd.scope]})
			}
		}
		if branch != nil {
			scopes = append(scopes, *branch)
		}
		if _, _, err := reg.Merge("", scopes...); err != nil {
			return err
		}
	}
	return nil
}

// loosens reports whether next is looser than old for spec: the merge of the two
// as consecutive scopes refuses a loosening. old nil means no stored row, so the
// key's default is the reference; a key with neither (an unset list key means
// "none allowed") counts any value as loosening. Any other failure counts as
// loosening too (fail closed).
func (w *Writer) loosens(keys *Keyset, spec KeySpec, old, next json.RawMessage) bool {
	reg, err := keys.registry()
	if err != nil {
		return true
	}
	if old == nil {
		if spec.Default == nil {
			return true
		}
		if old, err = json.Marshal(spec.Default); err != nil {
			return true
		}
	}
	_, _, err = reg.Merge("", trust.Scope{Kind: SourceUser, Values: map[string]json.RawMessage{spec.Key: old}},
		trust.Scope{Kind: SourceUser, Values: map[string]json.RawMessage{spec.Key: next}})
	return err != nil
}

// refuseMachine writes the refusal's audit row and returns the E687 error. A
// failed audit write does not turn the refusal into an allow.
func (w *Writer) refuseMachine(ctx context.Context, actor roles.Actor, op, target string) error {
	refusal := &roles.Error{Reason: ReasonMachineInvoked, Msg: "a machine-invoked caller (NSELF_INVOKED_BY) cannot loosen policy: " + op + " " + target}
	if w.Log == nil {
		return refusal
	}
	ev := audit.Event{Actor: actor.Label(), Action: "policy.refused", Target: target, Detail: "op=" + op + " reason=" + ReasonMachineInvoked}
	if err := w.Log.Append(ctx, ev); err != nil {
		return errors.Join(refusal, err)
	}
	return refusal
}
