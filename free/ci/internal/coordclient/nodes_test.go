package coordclient

// Purpose: tests of the node administration client (P7-NODE-26) against the real user API: a member reads
// and is refused every mutation with E664 role, an admin runs them, a failing command keeps its exit code
// and output, and the node id travels as one path segment.
// Inputs:  the sim of rig_test.go and a scripted clientapi.NodeRunner. Outputs: pass or fail.
// Constraints: the node routes register once per process; real TLS on 127.0.0.1.

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/roles"
)

var nodeRun struct {
	sync.Mutex
	exit  int
	calls []clientapi.NodeCommand
}

var registerNodes sync.Once

func nodeSim(t *testing.T) *sim {
	t.Helper()
	registerNodes.Do(func() {
		err := clientapi.RegisterNodeRoutes(func() clientapi.NodeRunner {
			return func(_ context.Context, _ string, c clientapi.NodeCommand) (clientapi.NodeResult, error) {
				nodeRun.Lock()
				defer nodeRun.Unlock()
				nodeRun.calls = append(nodeRun.calls, c)
				return clientapi.NodeResult{Exit: nodeRun.exit, Stdout: "out " + c.Verb, Stderr: "err"}, nil
			}
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	nodeRun.Lock()
	nodeRun.exit, nodeRun.calls = 0, nil
	nodeRun.Unlock()
	return newSim(t)
}

func tokenClient(s *sim, role roles.Role) *Client {
	return s.client(Creds{CA: s.rootFile(), TokenFile: s.tokenFile("pat", role, time.Hour, roles.AllProjects)})
}

func TestNodesClientMemberReadsAdminMutates(t *testing.T) {
	s := nodeSim(t)
	ctx := context.Background()
	member, admin := tokenClient(s, roles.Member), tokenClient(s, roles.Admin)
	if r, err := member.NodeList(ctx, true); err != nil || r.Stdout != "out list" {
		t.Fatalf("member list = %+v %v", r, err)
	}
	if r, err := member.NodeShow(ctx, "n1", false); err != nil || r.Stdout != "out show" {
		t.Fatalf("member show = %+v %v", r, err)
	}
	for _, verb := range []string{"set", "drain", "enable", "revoke", "remove", "rotate", "upgrade"} {
		if _, err := member.NodeDo(ctx, verb, "n1", clientapi.NodeRequest{Node: "n1"}); !reasonIs(err, clientapi.ReasonRole) {
			t.Fatalf("member %s: %v, want E664 role", verb, err)
		}
	}
	nodeRun.Lock()
	n := len(nodeRun.calls)
	nodeRun.Unlock()
	if n != 2 {
		t.Fatalf("runner saw %d calls, want only the 2 reads", n)
	}
	for _, verb := range []string{"drain", "enable", "revoke", "remove"} {
		if r, err := admin.NodeDo(ctx, verb, "n1", clientapi.NodeRequest{}); err != nil || r.Stdout != "out "+verb || r.Exit != 0 {
			t.Fatalf("admin %s = %+v %v", verb, r, err)
		}
	}
	if r, err := admin.NodeDo(ctx, "upgrade", "", clientapi.NodeRequest{All: true, Version: "1.5.0"}); err != nil || r.Stdout != "out upgrade" {
		t.Fatalf("admin upgrade = %+v %v", r, err)
	}
	last := nodeRun.calls[len(nodeRun.calls)-1]
	if last.Verb != "upgrade" || last.Node != "" || len(last.Args) != 2 {
		t.Fatalf("upgrade command %+v", last)
	}
}

func TestNodesClientFailedCommandKeepsExit(t *testing.T) {
	s := nodeSim(t)
	nodeRun.Lock()
	nodeRun.exit = 3
	nodeRun.Unlock()
	r, err := tokenClient(s, roles.Admin).NodeDo(context.Background(), "revoke", "n1", clientapi.NodeRequest{})
	if err != nil || r.Exit != 3 || r.Stderr != "err" {
		t.Fatalf("got %+v %v", r, err)
	}
}

func TestNodesClientIDIsOnePathSegment(t *testing.T) {
	s := nodeSim(t)
	_, err := tokenClient(s, roles.Admin).NodeDo(context.Background(), "revoke", "a/../b", clientapi.NodeRequest{})
	if !reasonIs(err, clientapi.ReasonNotFound) && !reasonIs(err, clientapi.ReasonBadRequest) {
		t.Fatalf("got %v", err)
	}
	nodeRun.Lock()
	defer nodeRun.Unlock()
	if len(nodeRun.calls) != 0 {
		t.Fatalf("a crafted id reached the runner: %+v", nodeRun.calls)
	}
}

func TestNodesClientNoCredential(t *testing.T) {
	s := nodeSim(t)
	if _, err := New(s.target(Creds{CA: s.rootFile()})); !reasonIs(err, clientapi.ReasonNoCredential) {
		t.Fatalf("got %v", err)
	}
}

func TestNodesClientLiftsTheHeaderTimeoutOnACopy(t *testing.T) {
	s := nodeSim(t)
	cl := tokenClient(s, roles.Admin)
	orig := cl.hc.Transport.(*http.Transport)
	if orig.ResponseHeaderTimeout == 0 {
		t.Fatal("the shared transport lost its header timeout")
	}
	if got := cl.nodeHTTP().Transport.(*http.Transport); got == orig || got.ResponseHeaderTimeout != 0 {
		t.Fatalf("node client transport: shared=%v timeout=%v", got == orig, got.ResponseHeaderTimeout)
	}
}
