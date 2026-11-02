package runners

import "context"

// HandleRunning obtains the coordinator's registrar and verifies the running job before pausing.
// Inputs: an authenticated, identity-bound webhook event. Outputs: whether a runner was paused.
// Constraints: API failures propagate so the delivery can be retried; OnRunning verifies ownership.
func HandleRunning(ctx context.Context, ev RunningEvent) (bool, error) {
	r, err := (&Minter{}).registrarIfConfigured(ctx)
	if err != nil {
		return false, err
	}
	if r == nil {
		return false, nil
	}
	return r.OnRunning(ctx, ev)
}
