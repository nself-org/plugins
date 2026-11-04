package agentapi

// Purpose: the welcome's contention block (D-1226): an agent that lists protocol.CompatContention gets the
//          node's stored policy; an agent that does not gets no block (an older agent decodes welcome
//          strictly); a node with no policy gets an empty block.
// Inputs:  the rig. Outputs: pass or fail.

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func welcomeOf(t *testing.T, a *agent, mut func(*protocol.Hello)) protocol.Welcome {
	t.Helper()
	resp := a.hello(mut)
	defer resp.Body.Close()
	w, ok := readFrame(t, resp.Body).Body.(*protocol.Welcome)
	if !ok || resp.StatusCode != 200 {
		t.Fatalf("status %d, body %T", resp.StatusCode, w)
	}
	return *w
}

func TestHelloDeliversTheContentionPolicyOnlyWhenAsked(t *testing.T) {
	policy := &protocol.NodePolicy{Node: "n-1", ReservedCPU: 2, Interactive: "protect", OnBattery: "pause", Persistence: "intermittent", Version: 3}
	r := newRig(t, func(cfg *Config) {
		cfg.Policy = func(_ context.Context, node string) (*protocol.NodePolicy, error) {
			if node == "n-1" {
				return policy, nil
			}
			return nil, nil
		}
	})
	asks := func(h *protocol.Hello) {
		h.CompatModeSupported = append(h.CompatModeSupported, protocol.CompatContention)
	}
	a := r.newAgent("n-1")
	if w := welcomeOf(t, a, nil); w.Contention != nil {
		t.Fatalf("an agent that did not ask got a contention block: %+v", w.Contention)
	}
	if w := welcomeOf(t, a, asks); w.Contention == nil || w.Contention.Policy == nil || *w.Contention.Policy != *policy {
		t.Fatalf("welcome contention = %+v, want the stored policy", w.Contention)
	}
	if w := welcomeOf(t, r.newAgent("n-2"), asks); w.Contention == nil || w.Contention.Policy != nil {
		t.Fatalf("a node with no policy: contention = %+v, want an empty block", w.Contention)
	}
}
