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
	"reflect"
	"slices"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
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

// policyReader is the stored-row read the write checks use: a *store.Tx, so the
// rows are the ones the write lands on.
type policyReader interface {
	PolicyRows(ctx context.Context, scope, subject string) ([]store.PolicyRow, error)
	PolicySubjects(ctx context.Context, scope string) ([]string, error)
}

// rowSet is the stored rows of one subject.
type rowSet struct {
	subject string
	rows    map[string]json.RawMessage
}

// rowSets holds, per scope, every subject a resolve could merge for that scope.
type rowSets map[string][]rowSet

// rowsOf reads the rows a resolve touched by the write could merge. The scope the
// write lands on has its one target subject. Every other scope lists all of its
// subjects (the current project and team included even without rows), so a user
// or team write is checked against every project row and a project write against
// every team row, not only the current repository's (D-1284).
func rowsOf(ctx context.Context, rd policyReader, t target, c Change) (rowSets, error) {
	team, err := teamOf(c.Team)
	if err != nil {
		return nil, err
	}
	out := rowSets{}
	for _, scope := range []string{"user", "team", "project"} {
		subjects := []string{t.subject}
		if scope != t.scope {
			switch scope {
			case "user":
				subjects = []string{LocalSubject}
			default:
				listed, err := rd.PolicySubjects(ctx, scope)
				if err != nil {
					return nil, fmt.Errorf("list %s policy: %w", scope, err)
				}
				current := team
				if scope == "project" {
					current = c.Project
				}
				if current != "" && !slices.Contains(listed, current) {
					listed = append(listed, current)
				}
				subjects = listed
			}
		}
		if len(subjects) == 0 {
			subjects = []string{""} // no subject has rows: one empty set
		}
		for _, subject := range subjects {
			set := rowSet{subject: subject, rows: map[string]json.RawMessage{}}
			if subject != "" {
				stored, err := rd.PolicyRows(ctx, scope, subject)
				if err != nil {
					return nil, fmt.Errorf("read %s policy: %w", scope, err)
				}
				for _, r := range stored {
					set.rows[r.Key] = r.Value
				}
			}
			out[scope] = append(out[scope], set)
		}
	}
	return out, nil
}

// put sets key in the row set of the scope and subject the write lands on.
func (rs rowSets) put(t target, key string, value json.RawMessage) {
	for _, set := range rs[t.scope] {
		if set.subject == t.subject {
			set.rows[key] = value
		}
	}
}

// storedOf is the row a write replaces (nil when there is none).
func storedOf(ctx context.Context, rd policyReader, t target, key string) json.RawMessage {
	rows, err := rd.PolicyRows(ctx, t.scope, t.subject)
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

// checkRows merges the operator rows and the protected-branch scope over the
// strictest default scopes the resolver can start from (an untrusted source on a
// private and on a public repository), for every user, team and project subject
// combination a resolve could pick. The protected-branch scope belongs to the
// current repository, so it joins only the combinations of branchProject.
func (k *Keyset) checkRows(rows rowSets, branch *trust.Scope, branchProject string) error {
	reg, err := k.registry()
	if err != nil {
		return err
	}
	kinds := []struct {
		scope string
		kind  trust.Source
	}{{"user", SourceUser}, {"team", SourceTeam}, {"project", SourceProject}}
	var walk func(i int, picked []rowSet) error
	walk = func(i int, picked []rowSet) error {
		if i < len(kinds) {
			for _, set := range rows[kinds[i].scope] {
				if err := walk(i+1, append(picked, set)); err != nil {
					return err
				}
			}
			return nil
		}
		for _, vis := range []string{"private", "public"} {
			scopes := []trust.Scope{trust.DefaultPolicy(vis)}
			for j, set := range picked {
				if len(set.rows) > 0 {
					scopes = append(scopes, trust.Scope{Kind: kinds[j].kind, Values: set.rows})
				}
			}
			if branch != nil && picked[2].subject == branchProject {
				scopes = append(scopes, *branch)
			}
			if _, _, err := reg.Merge("", scopes...); err != nil {
				if picked[2].subject != branchProject && picked[2].subject != "" {
					err = fmt.Errorf("%w (checked against project %q)", err, picked[2].subject)
				}
				return err
			}
		}
		return nil
	}
	return walk(0, make([]rowSet, 0, len(kinds)))
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
	// A TightenAny key (every spend key) has no stricter direction: Merge accepts any value, so a
	// merge check could never refuse it. For these keys any change from the stored value loosens,
	// because a machine-invoked caller must not turn paid placement on or raise a budget (TM-37, TM-38).
	if spec.Kind == trust.TightenAny {
		var a, b any
		return json.Unmarshal(old, &a) != nil || json.Unmarshal(next, &b) != nil || !reflect.DeepEqual(a, b)
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
