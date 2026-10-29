// Package pools resolves runner pools (Epic AUTO D4, D10) and the effective pool set of a project.
package pools

// Purpose: contract:ci.pool v1 (schema free/ci/schemas/pool.v1.schema.json): the resolved view of one
//          pool. A pool is not stored anywhere: it is the set of runners carrying the coordinator-assigned
//          label `pool.<id>` (P7-NODE-30), plus the implicit pools `local` (this machine's runner) and
//          `hosted` (every provider runner).
// Inputs:  none.
// Outputs: Pool, Member, Effective and the Scope and member Source names.
// Constraints: scope is derived from where a member was reached, never configured; a new field here is a
//          contract change.

import "github.com/nself-org/plugins/free/ci/internal/model"

// SchemaID is the value of Pool.Schema.
const SchemaID = "ci.pool/v1"

// Scope says whose authority a pool's membership comes from.
type Scope string

// The pool scopes.
const (
	// ScopeOperator is a pool of nodes in the invoking person's own store (and the local pool).
	ScopeOperator Scope = "operator"
	// ScopeProject is a pool with a member reached through a TEAM coordinator.
	ScopeProject Scope = "project"
	// ScopeHosted is the implicit pool of provider runners.
	ScopeHosted Scope = "hosted"
)

// Where a member was reached.
const (
	// SourceLocal is this machine's runner.
	SourceLocal = "local"
	// SourceRegistry is a node of the invoking person's store.
	SourceRegistry = "registry"
	// SourceCoordinator is a node reached through a TEAM coordinator.
	SourceCoordinator = "coordinator"
	// SourceProvider is a hosted provider runner.
	SourceProvider = "provider"
)

// The pool.excluded details, one per term of the effective set.
const (
	// DetailNotInProjectList: no pool of the runner is listed in pools.allow.
	DetailNotInProjectList = "not_in_project_list"
	// DetailNotEnrolled: the person is enrolled in no pool of the runner that the project allows.
	DetailNotEnrolled = "not_enrolled"
	// DetailDevNotAllowed: a dev run, and no remaining pool of the runner is listed in pools.dev.
	DetailDevNotAllowed = "dev_not_allowed"
)

// Member is one runner of a pool.
type Member struct {
	RunnerID string `json:"runner_id"`
	Location string `json:"location"`
	Source   string `json:"source"`
}

// Effective says whether the pool is in the effective set and, if not, why.
type Effective struct {
	Allowed bool                          `json:"allowed"`
	Reasons []model.PlacementReasonDetail `json:"reasons"`
}

// Pool is the contract:ci.pool v1 document of one pool.
type Pool struct {
	Schema    string    `json:"schema"`
	ID        string    `json:"id"`
	Scope     Scope     `json:"scope"`
	Members   []Member  `json:"members"`
	Effective Effective `json:"effective"`
}
