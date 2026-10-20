package burst

// Purpose: the production NodeRevoker: it revokes a burst node through the node
// lifecycle (NODE-05), whose registered revoker hooks run in the same
// transaction (the deny-list hook of NODE-07, and the store's burst hook that
// marks the intent tearing-down).
// Inputs:  a lifecycle.Deps. Outputs: a NodeRevoker.
// Constraints: a node that is already gone is success (revoke is idempotent here);
// any other failure is returned and the teardown keeps retrying.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

// LifecycleRevoker revokes nodes through lifecycle.Deps.Revoke.
type LifecycleRevoker struct{ Deps *lifecycle.Deps }

// Revoke implements NodeRevoker.
func (r LifecycleRevoker) Revoke(ctx context.Context, nodeID string) error {
	_, err := r.Deps.Revoke(ctx, nodeID)
	if err != nil && lifecycle.CodeOf(err) == lifecycle.CodeNotFound {
		return nil
	}
	return err
}

var _ NodeRevoker = LifecycleRevoker{}
