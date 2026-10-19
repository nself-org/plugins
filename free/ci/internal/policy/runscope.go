package policy

// Purpose: the per-run policy scope (P7-AUTO-29, debt D-0984): the values of
// the flags of one invocation (`--prefer`, `--runner`, `--max-queue`, later
// `--where`) reach the effective policy.
// Inputs: Revision.Run, a flat key to raw JSON map (schedpolicy.Flags.Values).
// Outputs: the trust.Scope the resolver merges last; trust.Merge decides what
// applies and lists the rest in Resolved.Ignored with a reason.
// Constraints: pure; an empty map is no scope, so a run without per-run values
// resolves to the exact digest it had before; the run scope never carries a
// Spend key (Merge ignores it with E671) and never loosens another scope
// (only sched.preset, a preference, takes any value of its closed set).

import (
	"encoding/json"

	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// runScope returns the run scope for values, or nil when there are none.
func runScope(values map[string]json.RawMessage) *trust.Scope {
	if len(values) == 0 {
		return nil
	}
	cp := make(map[string]json.RawMessage, len(values))
	for k, v := range values {
		cp[k] = append(json.RawMessage(nil), v...)
	}
	return &trust.Scope{Kind: trust.SourceRun, Values: cp}
}
