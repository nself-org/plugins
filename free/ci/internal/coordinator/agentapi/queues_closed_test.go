package agentapi

import (
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"testing"
	"time"
)

func TestQueuesDoNotRecreateClosedNode(t *testing.T) {
	for _, op := range []string{"pop lease", "pop control", "push lease", "push control"} {
		t.Run(op, func(t *testing.T) {
			s := &Server{cfg: Config{Now: time.Now}, nodes: map[string]*nodeState{}}
			sess := s.startSession("node")
			s.CloseNode("node")
			switch op {
			case "pop lease":
				s.popLease("node", sess)
			case "pop control":
				s.popControl("node")
			case "push lease":
				s.pushBackLease("node", protocol.Lease{})
			case "push control":
				s.pushBackControl("node", protocol.Message{})
			}
			if len(s.nodes) != 0 {
				t.Fatal("queue operation recreated a closed node")
			}
		})
	}
}
