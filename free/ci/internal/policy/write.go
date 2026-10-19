package policy

// Purpose: the write side of contract:ci.policy v1 (P7-TRUST-26, Epic D15): set,
// unset, pin and clear-pin of the operator policy rows. Every write is
// authorized as admin first, then written with its audit row in one
// transaction (store.ApplyWrite + audit.AppendTx).
// Inputs: a Change, the resolved Actor, a Store, an audit Log and a Runner.
// Outputs: a Result naming what was written (a Secret key's value is masked), or
// an error: E687 (not authorized, or reason machine_invoked), E672 (unknown key,
// bad value, unreachable pin), E670 (the row would loosen a parent scope), E651
// (bad arguments).
// Constraints: nothing is written when any check fails. The new rows must merge
// cleanly with the stored ones over the strictest default scopes, so a write can
// never make Resolve fail. With NSELF_INVOKED_BY set (Writer.Machine) a write
// that loosens a `trust.*` key, any unset of one, and any pin or clear-pin is
// refused with E687 reason machine_invoked and one audit row of the refusal, so
// a capped caller cannot lift its own cap (Epic D27, review F2). No Secret value
// reaches an audit row, a Result or an error.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// ReasonMachineInvoked is the E687 reason of a refused machine-invoked write.
const ReasonMachineInvoked = "machine_invoked"

// Masked stands in for the value of a Secret key.
const Masked = "[REDACTED]"

// The write operations.
const (
	OpSet   = "set"
	OpUnset = "unset"
	OpPin   = "pin"
	OpClear = "clear"
)

// Change is one requested write.
type Change struct {
	Op      string
	Scope   string          // user, team or project (set, unset)
	Team    string          // subject of the team scope
	Key     string          // set, unset
	Value   json.RawMessage // set: the JSON value
	SHA     string          // pin: a full commit id
	Root    string          // repository root (pin reachability)
	Project string          // project subject: ProjectIDGit of Root
}

// Result says what a write changed. Value is masked for a Secret key.
type Result struct {
	Op, Scope, Subject, Key, Value, SHA string
}

// Writer applies Changes.
type Writer struct {
	Store   *store.Store
	Log     *audit.Log // refusal rows
	Keys    *Keyset    // nil means Default
	Run     Runner     // pin reachability
	Machine bool       // NSELF_INVOKED_BY is set
	// Authorize is the role check; nil means roles.Authorize.
	Authorize func(ctx context.Context, a roles.Actor, role roles.Role, project string) error
}

func usage(format string, args ...any) error { return fmt.Errorf("E651 "+format, args...) }

type target struct{ scope, subject, authProject string }

// Apply authorizes and writes c.
func (w *Writer) Apply(ctx context.Context, actor roles.Actor, c Change) (Result, error) {
	keys := w.Keys
	if keys == nil {
		keys = Default
	}
	t, err := w.target(c)
	if err != nil {
		return Result{}, err
	}
	auth := w.Authorize
	if auth == nil {
		auth = roles.Authorize
	}
	if err := auth(ctx, actor, roles.Admin, t.authProject); err != nil {
		return Result{}, err
	}
	switch c.Op {
	case OpSet, OpUnset:
		return w.row(ctx, keys, actor, c, t)
	default:
		return w.pin(ctx, actor, c, t)
	}
}

func (w *Writer) target(c Change) (target, error) {
	switch c.Op {
	case OpPin, OpClear:
		if c.Project == "" {
			return target{}, usage("a pin needs a project")
		}
		return target{"project", c.Project, c.Project}, nil
	case OpSet, OpUnset:
	default:
		return target{}, usage("unknown policy operation %q", c.Op)
	}
	if c.Key == "" {
		return target{}, usage("a policy key is required")
	}
	switch c.Scope {
	case "user":
		return target{"user", LocalSubject, ""}, nil
	case "team":
		if c.Team == "" {
			return target{}, usage("--scope team needs --team <id>")
		}
		return target{"team", c.Team, ""}, nil
	case "project":
		if c.Project == "" {
			return target{}, usage("--scope project needs a project")
		}
		return target{"project", c.Project, c.Project}, nil
	}
	return target{}, usage("--scope must be user, team or project")
}

func (w *Writer) row(ctx context.Context, keys *Keyset, actor roles.Actor, c Change, t target) (Result, error) {
	spec, ok := keys.Find(c.Key)
	if !ok {
		return Result{}, refuse("unknown policy key %s", c.Key)
	}
	res := Result{Op: c.Op, Scope: t.scope, Subject: t.subject, Key: c.Key}
	pw := store.PolicyWrite{Op: store.PolicyUnset, Scope: t.scope, Subject: t.subject, Key: c.Key, Actor: actor.Label()}
	var value json.RawMessage
	if c.Op == OpSet {
		var err error
		if value, err = checkValue(spec, c.Value); err != nil {
			return Result{}, err
		}
		pw.Op, pw.Value = store.PolicySet, value
		res.Value = shown(spec, value)
	}
	if w.Machine && strings.HasPrefix(c.Key, "trust.") && (c.Op == OpUnset || w.loosens(keys, spec, w.stored(ctx, t, c.Key), value)) {
		return Result{}, w.refuseMachine(ctx, actor, c.Op, t.scope+"/"+c.Key)
	}
	if c.Op == OpSet { // an unset cannot make the remaining rows loosen a parent
		rows, err := w.rows(ctx, t, c)
		if err != nil {
			return Result{}, err
		}
		rows[t.scope][c.Key] = value
		if err := keys.checkRows(rows); err != nil {
			return Result{}, err
		}
	}
	detail := fmt.Sprintf("scope=%s subject=%s", t.scope, t.subject)
	if c.Op == OpSet {
		detail += " value=" + res.Value
	}
	ev := audit.Event{Actor: actor.Label(), Action: "policy." + c.Op, Target: c.Key, Detail: detail}
	return res, w.Store.ApplyWrite(ctx, pw, func(tx *store.Tx) error { return audit.AppendTx(tx, ev) })
}
