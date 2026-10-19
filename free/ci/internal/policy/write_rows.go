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

// checkRows merges the operator rows over the strictest default scopes the
// resolver can start from (an untrusted source on a private and on a public
// repository), so a stored row never makes Resolve fail with E670.
func (k *Keyset) checkRows(rows map[string]map[string]json.RawMessage) error {
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
		if _, _, err := reg.Merge("", scopes...); err != nil {
			return err
		}
	}
	return nil
}

// loosens reports whether next is looser than old for spec: the merge of the two
// as consecutive scopes refuses a loosening. old nil means no stored row, so the
// key's default is the reference; a key with neither cannot be loosened by being
// set. Any other failure counts as loosening (fail closed).
func (w *Writer) loosens(keys *Keyset, spec KeySpec, old, next json.RawMessage) bool {
	reg, err := keys.registry()
	if err != nil {
		return true
	}
	if old == nil {
		if spec.Default == nil {
			return false
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
	refusal := &roles.Error{Reason: ReasonMachineInvoked, Msg: "a machine-invoked caller (NSELF_INVOKED_BY) cannot loosen trust policy: " + op + " " + target}
	if w.Log == nil {
		return refusal
	}
	ev := audit.Event{Actor: actor.Label(), Action: "policy.refused", Target: target, Detail: "op=" + op + " reason=" + ReasonMachineInvoked}
	if err := w.Log.Append(ctx, ev); err != nil {
		return errors.Join(refusal, err)
	}
	return refusal
}
