package coordclient

// Purpose: tests of the labels and projects client (P7-NODE-30) against the real user API: a member reads
// and is refused every mutation with E664 role, an admin edits, the typed request reaches the runner and
// the node id stays one path segment.
// Inputs:  the sim of rig_test.go and a scripted clientapi.PlacementRunner. Outputs: pass or fail.
// Constraints: the placement routes register once per process; real TLS on 127.0.0.1.

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/roles"
)

var placementRun struct {
	sync.Mutex
	calls []clientapi.PlacementCommand
}

var registerPlacement sync.Once

func placementSim(t *testing.T) *sim {
	t.Helper()
	registerPlacement.Do(func() {
		err := clientapi.RegisterPlacementRoutes(func() clientapi.PlacementRunner {
			return func(_ context.Context, _ string, c clientapi.PlacementCommand) (clientapi.NodeResult, error) {
				placementRun.Lock()
				defer placementRun.Unlock()
				placementRun.calls = append(placementRun.calls, c)
				return clientapi.NodeResult{Stdout: "out " + c.Verb}, nil
			}
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	placementRun.Lock()
	placementRun.calls = nil
	placementRun.Unlock()
	return newSim(t)
}

func TestPlacementClientMemberReadsAdminEdits(t *testing.T) {
	s := placementSim(t)
	ctx := context.Background()
	member, admin := tokenClient(s, roles.Member), tokenClient(s, roles.Admin)
	if r, err := member.NodeProjects(ctx, "vps-a", false, clientapi.PlacementRequest{JSON: true}); err != nil || r.Stdout != "out projects list" {
		t.Fatalf("member projects list = %+v %v", r, err)
	}
	if r, err := member.NodeLabels(ctx, "mini1", false, clientapi.PlacementRequest{}); err != nil || r.Stdout != "out label list" {
		t.Fatalf("member label list = %+v %v", r, err)
	}
	if _, err := member.NodeLabels(ctx, "mini1", true, clientapi.PlacementRequest{Labels: []string{"xcode"}}); !reasonIs(err, clientapi.ReasonRole) {
		t.Fatalf("member label edit: %v, want E664 role", err)
	}
	if _, err := member.NodeProjects(ctx, "vps-a", true, clientapi.PlacementRequest{Action: "add", Projects: []string{"p"}}); !reasonIs(err, clientapi.ReasonRole) {
		t.Fatalf("member projects add: %v, want E664 role", err)
	}
	if r, err := admin.NodeLabels(ctx, "mini1", true, clientapi.PlacementRequest{Labels: []string{"xcode", "arch=arm64"}, Remove: []string{"gpu"}}); err != nil || r.Stdout != "out label" {
		t.Fatalf("admin label = %+v %v", r, err)
	}
	if r, err := admin.NodeProjects(ctx, "vps-a", true, clientapi.PlacementRequest{Action: "remove", Projects: []string{"local:abc"}}); err != nil || r.Stdout != "out projects remove" {
		t.Fatalf("admin projects remove = %+v %v", r, err)
	}
	placementRun.Lock()
	defer placementRun.Unlock()
	if len(placementRun.calls) != 4 {
		t.Fatalf("runner saw %d calls, want the 2 reads and 2 admin edits: %+v", len(placementRun.calls), placementRun.calls)
	}
	if c := placementRun.calls[2]; c.Node != "mini1" || !reflect.DeepEqual(c.Labels, []string{"xcode", "arch=arm64"}) || !reflect.DeepEqual(c.Remove, []string{"gpu"}) {
		t.Fatalf("label command %+v", c)
	}
}

func TestPlacementClientIDIsOnePathSegment(t *testing.T) {
	s := placementSim(t)
	_, err := tokenClient(s, roles.Admin).NodeLabels(context.Background(), "a/../b", true, clientapi.PlacementRequest{Labels: []string{"x"}})
	if !reasonIs(err, clientapi.ReasonNotFound) && !reasonIs(err, clientapi.ReasonBadRequest) {
		t.Fatalf("got %v", err)
	}
	placementRun.Lock()
	defer placementRun.Unlock()
	if len(placementRun.calls) != 0 {
		t.Fatalf("a crafted id reached the runner: %+v", placementRun.calls)
	}
}
