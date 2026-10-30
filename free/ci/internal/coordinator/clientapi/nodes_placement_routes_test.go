package clientapi

// Purpose: tests of the labels and projects routes (P7-NODE-30): a member lists but gets E664 role on
// every mutation, an admin runs each with typed fields, a project-scoped caller is refused, and bad input
// never reaches the runner.
// Inputs:  the rig of rig_test.go and a scripted PlacementRunner. Outputs: pass or fail.
// Constraints: the routes register once per process, so one runner variable is shared by all tests.

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/roles"
)

type placementCall struct {
	actor string
	cmd   PlacementCommand
}

var placementFake struct {
	sync.Mutex
	calls []placementCall
}

var registerPlacementOnce sync.Once

func placementRig(t *testing.T) *rig {
	t.Helper()
	registerPlacementOnce.Do(func() {
		err := RegisterPlacementRoutes(func() PlacementRunner {
			return func(_ context.Context, actor string, cmd PlacementCommand) (NodeResult, error) {
				placementFake.Lock()
				defer placementFake.Unlock()
				placementFake.calls = append(placementFake.calls, placementCall{actor, cmd})
				return NodeResult{Stdout: "ran " + cmd.Verb + "\n"}, nil
			}
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	placementFake.Lock()
	placementFake.calls = nil
	placementFake.Unlock()
	return newRig(t)
}

func placementCalls() []placementCall {
	placementFake.Lock()
	defer placementFake.Unlock()
	return append([]placementCall(nil), placementFake.calls...)
}

var placementMutations = []struct{ name, path, body string }{
	{"label", PathNodesPfx + "mini1/labels", `{"labels":["xcode","arch=arm64"],"remove":["old"]}`},
	{"projects add", PathNodesPfx + "vps-a/projects", `{"action":"add","projects":["local:abc"]}`},
	{"projects remove", PathNodesPfx + "vps-a/projects", `{"action":"remove","projects":["local:abc"]}`},
}

func TestPlacementMemberListsOnly(t *testing.T) {
	r := placementRig(t)
	tok := r.token("bob", roles.Member, time.Hour, roles.AllProjects)
	for _, m := range placementMutations {
		st, b := r.do(r.client(), "POST", m.path, tok, m.body)
		refused(t, st, b, 403, ReasonRole)
	}
	if got := placementCalls(); len(got) != 0 {
		t.Fatalf("a member ran %+v", got)
	}
	for path, verb := range map[string]string{PathNodesPfx + "vps-a/projects": VerbProjectsList, PathNodesPfx + "mini1/labels": VerbLabelList} {
		if st, b := r.do(r.client(), "GET", path+"?json=1", tok, ""); st != 200 {
			t.Fatalf("member GET %s = %d %s", path, st, b)
		}
		got := placementCalls()
		if last := got[len(got)-1]; last.cmd.Verb != verb || !last.cmd.JSON || !strings.Contains(last.actor, "bob") {
			t.Fatalf("member read ran %+v", last)
		}
	}
}

func TestPlacementAdminRunsEach(t *testing.T) {
	r := placementRig(t)
	tok := r.token("ada", roles.Admin, time.Hour, roles.AllProjects)
	for i, m := range placementMutations {
		st, b := r.do(r.client(), "POST", m.path, tok, m.body)
		var res NodeResult
		if st != 200 || json.Unmarshal(b, &res) != nil || res.Stdout != "ran "+m.name+"\n" {
			t.Fatalf("admin %s = %d %s", m.name, st, b)
		}
		if got := placementCalls(); len(got) != i+1 || !strings.Contains(got[i].actor, "ada") {
			t.Fatalf("admin %s ran %+v", m.name, got)
		}
	}
	c := placementCalls()
	if !reflect.DeepEqual(c[0].cmd.Labels, []string{"xcode", "arch=arm64"}) || !reflect.DeepEqual(c[0].cmd.Remove, []string{"old"}) || c[0].cmd.Node != "mini1" {
		t.Fatalf("label command %+v", c[0].cmd)
	}
	if c[1].cmd.Verb != VerbProjectsAdd || !reflect.DeepEqual(c[1].cmd.Projects, []string{"local:abc"}) {
		t.Fatalf("projects add command %+v", c[1].cmd)
	}
}

func TestPlacementScopedCallerRefused(t *testing.T) {
	r := placementRig(t)
	tok := r.token("dan", roles.Admin, time.Hour, "acme/app")
	for _, m := range placementMutations {
		st, b := r.do(r.client(), "POST", m.path, tok, m.body)
		refused(t, st, b, 403, ReasonProject)
	}
	mem := r.token("eve", roles.Member, time.Hour, "acme/app")
	st, b := r.do(r.client(), "GET", PathNodesPfx+"vps-a/projects", mem, "")
	refused(t, st, b, 403, ReasonProject)
	if got := placementCalls(); len(got) != 0 {
		t.Fatalf("a scoped caller ran %+v", got)
	}
}

func TestPlacementRejectsBadInput(t *testing.T) {
	r := placementRig(t)
	tok := r.token("ada", roles.Admin, time.Hour, roles.AllProjects)
	for _, tc := range []struct{ name, path, body string }{
		{"unknown field", PathNodesPfx + "n1/labels", `{"argv":["--x"]}`},
		{"flag as label", PathNodesPfx + "n1/labels", `{"labels":["--coordinator=x"]}`},
		{"space in label", PathNodesPfx + "n1/labels", `{"labels":["a b"]}`},
		{"projects on labels", PathNodesPfx + "n1/labels", `{"projects":["p"]}`},
		{"labels on projects", PathNodesPfx + "n1/projects", `{"action":"add","labels":["x"]}`},
		{"no action", PathNodesPfx + "n1/projects", `{"projects":["p"]}`},
		{"bad action", PathNodesPfx + "n1/projects", `{"action":"list"}`},
		{"directory as project", PathNodesPfx + "n1/projects", `{"action":"add","projects":["../x"]}`},
		{"all on labels", PathNodesPfx + "n1/labels", `{"all":true}`},
		{"bad node id", PathNodesPfx + "..%2fx/labels", `{}`},
		{"too many", PathNodesPfx + "n1/labels", `{"labels":["` + strings.Repeat(`a","`, maxPlacementItems) + `a"]}`},
		{"not json", PathNodesPfx + "n1/labels", `{`},
		{"trailing data", PathNodesPfx + "n1/labels", `{} {}`},
		{"too long", PathNodesPfx + "n1/labels", `{"labels":["` + strings.Repeat("a", maxPlacementBody) + `"]}`},
	} {
		st, b := r.do(r.client(), "POST", tc.path, tok, tc.body)
		if st != 400 && st != 404 {
			t.Errorf("%s: got %d %s", tc.name, st, b)
		}
	}
	if got := placementCalls(); len(got) != 0 {
		t.Fatalf("bad input reached the runner: %+v", got)
	}
}
