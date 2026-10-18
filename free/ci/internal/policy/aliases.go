package policy

// Purpose: the contract:ci.policy v1 types. They live in the pure trust
// package; this file re-exports them so SCHED and TRUST register keys and read
// answers through one import (P7-TRUST-27, Epic D15).
// Inputs: none. Outputs: type aliases and the Source constants.
// Constraints: aliases only, no behaviour; a new name here is a contract change.

import "github.com/nself-org/plugins/free/ci/internal/trust"

// KeySpec describes one policy key (or one `prefix.*` family).
type KeySpec = trust.KeySpec

// Tightening says how a lower scope may change a key.
type Tightening = trust.Tightening

// Effective is the merged policy: Value and Ignored read it.
type Effective = trust.Effective

// Ignored is one field a scope was not allowed to set.
type Ignored = trust.Ignored

// Source names the scope a value came from.
type Source = trust.Source

// Digest is the sha256 hex of the canonical effective policy and the pin.
type Digest = trust.Digest

// The tightening kinds, as in trust.
const (
	TightenAny      = trust.TightenAny
	TightenLower    = trust.TightenLower
	TightenHigher   = trust.TightenHigher
	TightenSubset   = trust.TightenSubset
	TightenSuperset = trust.TightenSuperset
	TightenOnlyOn   = trust.TightenOnlyOn
	TightenFixed    = trust.TightenFixed
)

// The scopes a value can come from.
const (
	SourceDefault         = trust.SourceDefault
	SourceUser            = trust.SourceUser
	SourceTeam            = trust.SourceTeam
	SourceProject         = trust.SourceProject
	SourceProtectedBranch = trust.SourceProtectedBranch
	SourceRevision        = trust.SourceRevision
)
