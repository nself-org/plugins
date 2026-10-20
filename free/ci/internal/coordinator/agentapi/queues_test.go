package agentapi

// Purpose: TestClosedNodeIsNotRecreatedByQueues (D-1308): a handler that still holds a popped lease or
//          control message after CloseNode deleted the node's entry must not bring the entry back.
// Inputs:  the rig. Outputs: pass or fail.
// Constraints: no node is registered; any entry in the map after the calls was created by a queue call.

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func TestClosedNodeIsNotRecreatedByQueues(t *testing.T) {
	r := newRig(t, nil)
	r.srv.CloseNode("gone")
	if l, _ := r.srv.popLease("gone", &session{epochs: map[string]uint64{}}); l != nil {
		t.Fatalf("lease from a closed node: %+v", l)
	}
	r.srv.pushBackLease("gone", protocol.Lease{LeaseID: "l-1"})
	if m, _ := r.srv.popControl("gone"); m != nil {
		t.Fatalf("control message from a closed node: %+v", m)
	}
	r.srv.pushBackControl("gone", protocol.Message{})
	r.srv.mu.Lock()
	defer r.srv.mu.Unlock()
	if ns, ok := r.srv.nodes["gone"]; ok {
		t.Fatalf("a queue call re-created the closed node entry: %+v", ns)
	}
}
