package update

// Purpose: the audit trail of an upgrade: one row per step (hq NODE D11, "every step is an audit row").
// Inputs:  the step name, the node id, a short detail of ids and versions.
// Outputs: rows in the store's audit table (action node.upgrade.<step>), written by the admin actor.
// Constraints: a detail holds versions, node ids and reason keys only, never a secret, a path of a key or command output; a row that
//          cannot be written fails the step that asked for it (an unaudited upgrade step does not happen).

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Steps of an upgrade, in order, and the outcomes around them.
const (
	StepVerify    = "verify"
	StepDrain     = "drain"
	StepWait      = "wait"
	StepInstall   = "install"
	StepRestart   = "restart"
	StepHello     = "hello"
	StepProbe     = "probe"
	StepEnable    = "enable"
	StepRollback  = "rollback"
	StepRefused   = "refused"
	StepFailed    = "failed"
	StepDowngrade = "downgrade"
	StepNoop      = "noop"
)

// Auditor records one step.
type Auditor interface {
	Step(ctx context.Context, step, node, detail string) error
}

// StoreAuditor writes steps to the CI store as Actor.
type StoreAuditor struct {
	Store *store.Store
	Actor string
}

// Step implements Auditor.
func (a StoreAuditor) Step(ctx context.Context, step, node, detail string) error {
	return a.Store.AuditAppend(ctx, store.AuditInput{Actor: a.Actor, Action: "node.upgrade." + step, Target: node, Detail: detail})
}
