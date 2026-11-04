package lifecycle

// Purpose: deliver a node's contention policy (`nodes set`, P7-NODE-13, D-1226) to a LAN node: the
//          policy is written to the agent's <state>/contention.json over the node's pinned ssh route by
//          `nself-ci-agent policy --set <base64url JSON>`, which the agent's gate reads on every lease.
// Inputs:  the registered node and its stored settings.
// Outputs: PolicyDelivery naming the route and the outcome.
// Constraints: a node with no ssh destination (a pull node, carrier B) is not dialled: its policy rides
//          on the next welcome (agentapi Config.Policy); a missing host-key pin is E655 and nothing is
//          dialled; the argument is base64url, so the remote shell sees no quoting.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Policy routes.
const (
	PolicyViaSSH     = "ssh"
	PolicyViaWelcome = "welcome"
)

// PolicyDelivery is how the policy reaches the node. Err is set when an ssh push failed.
type PolicyDelivery struct {
	Route     string `json:"route"`
	Delivered bool   `json:"delivered"`
	Error     string `json:"error,omitempty"`
}

// PushPolicy writes s to a LAN node, or reports that a pull node gets it in its next welcome.
func (d *Deps) PushPolicy(ctx context.Context, n model.Capability, s store.NodeSettings) PolicyDelivery {
	if _, err := specOf(n); err != nil {
		var le *Error
		if errors.As(err, &le) && le.Reason == "no_destination" {
			return PolicyDelivery{Route: PolicyViaWelcome}
		}
	}
	out := PolicyDelivery{Route: PolicyViaSSH}
	raw, err := json.Marshal(s)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	t, err := d.targetFor(ctx, n)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	if _, err = d.Run(ctx, t, agentPath, "policy", "--set", base64.RawURLEncoding.EncodeToString(raw)); err != nil {
		out.Error = transportErr("agent", "the agent did not take the policy", err).Error()
		return out
	}
	out.Delivered = true
	return out
}
