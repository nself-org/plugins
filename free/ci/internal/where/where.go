// Package where maps the `--where` flag to per-run policy values.
//
// Purpose: one pure function, Values, turns local|<node>|<pool>|hosted|auto|all
// into the revision-scope values of the run scope (P7-AUTO-29). There is no
// second placement path: the values go through sched.Place and dispatch.Admit
// like any other run-scope value.
// Inputs:  the flag text, what the operator has registered (Known) and the
// kind of run (Opts).
// Outputs: a Target (kind, values, notes) or an *Error carrying E797.
// Constraints: pure, no I/O, no clock. A flag value can only narrow: a pool or
// node outside the effective set is refused with the pool.excluded reason,
// never widened. `auto` leaves sched.preset to the operator (the sched.preset
// key admits only its closed preset names, so no constant is written here).
package where

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pools"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
)

// CodeInvalid is E797: an unknown --where value, or `all` before a race hook exists.
const CodeInvalid = "E797"

func init() {
	model.Register(model.Code{ID: CodeInvalid, Class: "usage", Summary: "--where value is unknown or not available",
		Fix: "Use local, a node id, a pool id, hosted, auto or all (all needs a race hook); list targets with `nself ci nodes pools list`"})
}

// Kinds of target.
const (
	KindLocal  = "local"
	KindNode   = "node"
	KindPool   = "pool"
	KindHosted = "hosted"
	KindAuto   = "auto"
	KindAll    = "all"
)

// ReasonPoolExcluded is the placement reason of a target outside the effective set.
const ReasonPoolExcluded = "pool.excluded"

// Known is what the operator has registered, as the caller read it.
type Known struct {
	Nodes     []string          // registered node ids (non-local)
	Pools     []string          // pool ids a runner belongs to
	Providers []string          // registered hosted provider ids
	Effective []string          // pools.allow after the merge; nil means every pool
	Excluded  map[string]string // node id -> reason it is outside the effective set
}

// Opts describes the run.
type Opts struct {
	Dev       bool // a dev run: pool narrowing goes to pools.dev, not pools.allow
	RaceReady bool // a RaceHook is registered (run.RaceRegistered)
}

// Target is the mapped flag.
type Target struct {
	Kind   string
	Values map[string]json.RawMessage
	Notes  []string
}

// Error is an E797 refusal with the reasons that explain it.
type Error struct {
	Message string
	Reasons []string
}

func (e *Error) Error() string {
	if len(e.Reasons) == 0 {
		return CodeInvalid + ": " + e.Message
	}
	return CodeInvalid + ": " + e.Message + " (" + strings.Join(e.Reasons, "; ") + ")"
}

func refuse(reasons []string, format string, a ...any) *Error {
	return &Error{Message: fmt.Sprintf(format, a...), Reasons: reasons}
}

// HasRemote reports whether the operator registered a non-local node or a provider.
func (k Known) HasRemote() bool { return len(k.Nodes) > 0 || len(k.Providers) > 0 }

func put(key string, v any) map[string]json.RawMessage {
	raw, _ := json.Marshal(v) // strings, bools and string lists always marshal
	return map[string]json.RawMessage{key: raw}
}

// Values maps one --where value. Names are matched in this order: the six
// reserved words, a registered node id, then a pool id.
func Values(flag string, k Known, o Opts) (Target, error) {
	switch flag {
	case pools.Local:
		return Target{Kind: KindLocal, Values: put(schedpolicy.KeyLocalOnly, true)}, nil
	case "hosted":
		if len(k.Providers) == 0 {
			return Target{}, refuse([]string{"no hosted provider is registered"}, "--where hosted has no target")
		}
		ids := slices.Clone(k.Providers)
		slices.Sort(ids)
		return Target{Kind: KindHosted, Values: put(schedpolicy.KeyProvidersAllow, ids)}, nil
	case "auto":
		return Target{Kind: KindAuto}, nil
	case "all":
		if !o.RaceReady {
			return Target{}, refuse(nil, "race not available")
		}
		return Target{Kind: KindAll}, nil
	case "":
		return Target{}, refuse(nil, "--where needs a value")
	}
	if slices.Contains(k.Nodes, flag) {
		if why, out := k.Excluded[flag]; out {
			return Target{}, refuse([]string{ReasonPoolExcluded + ": " + why}, "node %q is outside the effective set", flag)
		}
		return Target{Kind: KindNode, Values: put(schedpolicy.KeyRunner, flag)}, nil
	}
	if pools.ValidID(flag) && slices.Contains(k.Pools, flag) {
		if k.Effective != nil && !slices.Contains(k.Effective, flag) {
			return Target{}, refuse([]string{ReasonPoolExcluded + ": pool " + flag + " is not in pools.allow"}, "pool %q is outside the effective set", flag)
		}
		key := pools.KeyAllow
		if o.Dev {
			key = pools.KeyDev
		}
		return Target{Kind: KindPool, Values: put(key, []string{flag})}, nil
	}
	return Target{}, refuse(nil, "unknown value %q (local, a node id, a pool id, hosted, auto or all)", flag)
}
