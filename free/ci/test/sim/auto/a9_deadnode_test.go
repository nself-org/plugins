package auto

// Purpose: A9 status with one registered node at an unroutable address (dead-lab, TEST-NET
// 192.0.2.1) among healthy nodes: the node checks fail that node alone and the section exits red.
// Inputs: nodes/doctor.Evaluate over Facts built in memory. Outputs: pass or fail.
// Constraints: no dial happens; reachability is a fact the test supplies.

import (
	"errors"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/model"
	nd "github.com/nself-org/plugins/free/ci/internal/nodes/doctor"
)

func sshNodeOf(id, host string) nd.Node {
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Identity.ID, c.Identity.Name, c.Identity.Provider, c.Identity.Transport = id, id, "ssh", "ssh"
	c.Identity.SSH = &model.CapabilitySSH{Hostname: host, Port: 22, User: "ci"}
	return nd.Node{Cap: c, State: "online"}
}

func TestConformanceA9DeadNode(t *testing.T) {
	f := nd.Facts{
		Now:   time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC),
		Nodes: []nd.Node{sshNodeOf("alpha", "alpha.lan.test"), sshNodeOf("bravo", "bravo.lan.test"), sshNodeOf("dead-lab", "192.0.2.1")},
		Reach: map[string]error{"alpha": nil, "bravo": nil, "dead-lab": errors.New("dial tcp 192.0.2.1:22: i/o timeout")},
	}
	out := nd.Evaluate(f)
	fails := map[string]bool{}
	reach := 0
	for _, c := range out {
		if c.ID != nd.IDReach {
			continue
		}
		reach++
		if c.Status == doctor.Fail {
			fails[c.Subject] = true
		}
	}
	if reach != 3 || len(fails) != 1 || !fails["dead-lab"] {
		t.Fatalf("reachability rows %d, failing subjects %v", reach, fails)
	}
	if doctor.ExitCode(out) == doctor.ExitClean {
		t.Fatal("a dead node left the section clean")
	}
}
