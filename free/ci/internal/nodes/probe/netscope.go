package probe

// Purpose: record the network scopes a node proves it enforces (capability network.enforced_scopes, D-1332).
// Inputs:  the node's agent, which runs the scope proof on the node (`nself-ci-agent netscope-probe`, one JSON line
//          {"scopes":[...]}), and the capability the previous probe stored.
// Outputs: c.Network with a probe-sourced fact: the scopes, or unknown when the agent could not say.
// Constraints: the proof builds a job network and a sidecar, so it only runs on a node with docker or podman, and a
//          result younger than netscopeTTL is kept instead of probing again. Unreadable output or an unknown scope
//          is "unknown", never a guess. The registry still discards the fact below container isolation.

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// cmdNetscope runs from the SSH user's home (the same relative path `nodes add` uses for the agent).
const cmdNetscope = ".nself/ci-agent/bin/nself-ci-agent netscope-probe"

const netscopeTTL = 10 * time.Minute

func collectNetscope(ctx context.Context, c *model.Capability, runner commandRunner, now time.Time) {
	hasRuntime := c.Tools.Docker.Value != nil && *c.Tools.Docker.Value || c.Tools.Podman.Value != nil && *c.Tools.Podman.Value
	if !hasRuntime {
		c.Network = nil
		return
	}
	if n := c.Network; n != nil && n.EnforcedScopes.Value != nil {
		if age := now.Sub(n.EnforcedScopes.ObservedAt); age >= 0 && age < netscopeTTL {
			return
		}
	}
	out, err := runner(ctx, cmdNetscope)
	scopes := parseNetscope(out)
	if err != nil || ctx.Err() != nil {
		scopes = nil
	}
	c.Network = &model.CapabilityNetwork{EnforcedScopes: observed(scopes, now)}
}

// parseNetscope reads the agent's last output line; any malformed line or unknown scope gives nil.
func parseNetscope(out string) *[]model.NetworkScope {
	out = strings.TrimSpace(out)
	if i := strings.LastIndexByte(out, '\n'); i >= 0 {
		out = out[i+1:]
	}
	var r struct {
		Scopes []model.NetworkScope `json:"scopes"`
	}
	if json.Unmarshal([]byte(out), &r) != nil || r.Scopes == nil {
		return nil
	}
	for _, s := range r.Scopes {
		if !s.Valid() {
			return nil
		}
	}
	return &r.Scopes
}
