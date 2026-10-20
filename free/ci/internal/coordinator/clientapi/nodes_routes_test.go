package clientapi

// Purpose: tests of the /v1/nodes routes (P7-NODE-26): a member lists and shows but cannot run any of the
// seven mutations (E664 role), an admin can, a project-scoped admin cannot, add and pair have no route,
// and a request cannot turn a value into a second flag.
// Inputs:  the rig of rig_test.go and a scripted NodeRunner. Outputs: pass or fail.
// Constraints: the routes register once per process (RegisterRoute is global), so one runner variable
//          is shared by all tests; real SQLite, real pki, real TLS on 127.0.0.1.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/roles"
)

type nodeCall struct {
	actor string
	cmd   NodeCommand
}

var nodeFake struct {
	sync.Mutex
	on    bool
	exit  int
	calls []nodeCall
}

var registerNodeOnce sync.Once

func nodeRig(t *testing.T) *rig {
	t.Helper()
	registerNodeOnce.Do(func() {
		err := RegisterNodeRoutes(func() NodeRunner {
			nodeFake.Lock()
			defer nodeFake.Unlock()
			if !nodeFake.on {
				return nil
			}
			return func(_ context.Context, actor string, cmd NodeCommand) (NodeResult, error) {
				nodeFake.Lock()
				defer nodeFake.Unlock()
				nodeFake.calls = append(nodeFake.calls, nodeCall{actor, cmd})
				return NodeResult{Exit: nodeFake.exit, Stdout: "ran " + cmd.Verb + "\n"}, nil
			}
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	nodeFake.Lock()
	nodeFake.on, nodeFake.exit, nodeFake.calls = true, 0, nil
	nodeFake.Unlock()
	return newRig(t)
}

func nodeCalls() []nodeCall {
	nodeFake.Lock()
	defer nodeFake.Unlock()
	return append([]nodeCall(nil), nodeFake.calls...)
}

// mutations are the seven admin routes with a valid body each.
var mutations = []struct{ name, path, body string }{
	{"set", PathNodesPfx + "n1/set", `{"reserve_cpu":"50%"}`},
	{"drain", PathNodesPfx + "n1/drain", ``},
	{"enable", PathNodesPfx + "n1/enable", `{}`},
	{"revoke", PathNodesPfx + "n1/revoke", ``},
	{"remove", PathNodesPfx + "n1/remove", ``},
	{"rotate", PathNodesPfx + "n1/rotate", `{"host_key":"SHA256:abc/+=","mode":"host-key"}`},
	{"upgrade", PathNodesUpgrade, `{"node":"n1","version":"1.5.0"}`},
}

func TestNodesAuthzMemberReadsOnly(t *testing.T) {
	r := nodeRig(t)
	tok := r.token("bob", roles.Member, time.Hour, roles.AllProjects)
	c := r.client()
	for _, m := range mutations {
		st, b := r.do(c, "POST", m.path, tok, m.body)
		refused(t, st, b, 403, ReasonRole)
	}
	if got := nodeCalls(); len(got) != 0 {
		t.Fatalf("a member ran %d node commands: %+v", len(got), got)
	}
	for _, path := range []string{PathNodes, PathNodesPfx + "n1"} {
		st, b := r.do(c, "GET", path, tok, "")
		if st != 200 {
			t.Fatalf("member GET %s = %d %s", path, st, b)
		}
	}
	got := nodeCalls()
	if len(got) != 2 || got[0].cmd.Verb != "list" || got[1].cmd.Verb != "show" || got[1].cmd.Node != "n1" || !strings.Contains(got[0].actor, "bob") {
		t.Fatalf("member reads ran %+v", got)
	}
}

func TestNodesAuthzAdminRunsEach(t *testing.T) {
	r := nodeRig(t)
	tok := r.token("ada", roles.Admin, time.Hour, roles.AllProjects)
	c := r.client()
	for i, m := range mutations {
		st, b := r.do(c, "POST", m.path, tok, m.body)
		var res NodeResult
		if st != 200 || json.Unmarshal(b, &res) != nil || res.Stdout != "ran "+m.name+"\n" {
			t.Fatalf("admin %s = %d %s", m.name, st, b)
		}
		got := nodeCalls()
		if len(got) != i+1 || got[i].cmd.Verb != m.name || !strings.Contains(got[i].actor, "ada") || got[i].cmd.Node != "n1" {
			t.Fatalf("admin %s ran %+v", m.name, got)
		}
	}
	got := nodeCalls()
	if a := got[0].cmd.Args; len(a) != 1 || a[0] != "--reserve-cpu=50%" {
		t.Fatalf("set args %v", a)
	}
	if a := got[5].cmd.Args; len(a) != 2 || a[0] != "--host-key=SHA256:abc/+=" || a[1] != "--mode=host-key" {
		t.Fatalf("rotate args %v", a)
	}
	if a := got[6].cmd.Args; len(a) != 1 || a[0] != "--version=1.5.0" {
		t.Fatalf("upgrade args %v", a)
	}
}

func TestNodesAuthzAdminCertificate(t *testing.T) {
	r := nodeRig(t)
	if err := r.svc.Assign(context.Background(), r.op(), roles.Request{Principal: "carol", Role: roles.Admin, Projects: []string{roles.AllProjects}}); err != nil {
		t.Fatal(err)
	}
	st, b := r.do(r.client(r.userCert("carol")), "POST", PathNodesPfx+"n1/revoke", "", "")
	if st != 200 {
		t.Fatalf("admin certificate revoke = %d %s", st, b)
	}
	if got := nodeCalls(); len(got) != 1 || got[0].actor != "user:carol" {
		t.Fatalf("ran %+v", got)
	}
}

func TestNodesAuthzScopedAdminRefused(t *testing.T) {
	r := nodeRig(t)
	tok := r.token("dan", roles.Admin, time.Hour, "acme/app") // admin of one project is not a coordinator admin
	for _, m := range mutations {
		st, b := r.do(r.client(), "POST", m.path, tok, m.body)
		refused(t, st, b, 403, ReasonProject)
	}
	if got := nodeCalls(); len(got) != 0 {
		t.Fatalf("a scoped admin ran %+v", got)
	}
}

func TestNodesAuthzNoAddOrPairRoute(t *testing.T) {
	r := nodeRig(t)
	tok := r.token("ada", roles.Admin, time.Hour, roles.AllProjects)
	for _, verb := range []string{"add", "pair", "test", "provision"} {
		st, b := r.do(r.client(), "POST", PathNodesPfx+"n1/"+verb, tok, `{}`)
		refused(t, st, b, 404, ReasonNotFound)
	}
	if got := nodeCalls(); len(got) != 0 {
		t.Fatalf("ran %+v", got)
	}
}

func TestNodesAuthzRejectsBadInput(t *testing.T) {
	r := nodeRig(t)
	tok := r.token("ada", roles.Admin, time.Hour, roles.AllProjects)
	for _, tc := range []struct{ name, path, body string }{
		{"unknown field", PathNodesPfx + "n1/revoke", `{"argv":["--x"]}`},
		{"flag as value", PathNodesPfx + "n1/set", `{"reserve_cpu":"--agent-binary=/x"}`},
		{"value with a space", PathNodesPfx + "n1/set", `{"reserve_mem":"2 g"}`},
		{"field of another verb", PathNodesPfx + "n1/revoke", `{"reserve_cpu":"1"}`},
		{"upgrade field on set", PathNodesPfx + "n1/set", `{"version":"1.5.0"}`},
		{"bad node id", PathNodesPfx + "..%2fx/revoke", ``},
		{"space in the node id", PathNodesPfx + "a%20b/drain", ``},
		{"upgrade neither", PathNodesUpgrade, `{"version":"1.5.0"}`},
		{"upgrade both", PathNodesUpgrade, `{"node":"n1","all":true}`},
		{"not json", PathNodesPfx + "n1/drain", `{`},
		{"trailing data", PathNodesPfx + "n1/revoke", `{} {"x":1}`},
		{"trailing garbage", PathNodesPfx + "n1/revoke", `{}x`},
		{"too long", PathNodesPfx + "n1/revoke", `{"host_key":"` + strings.Repeat("a", maxNodeBody) + `"}`},
	} {
		st, b := r.do(r.client(), "POST", tc.path, tok, tc.body)
		if st != 400 && st != 404 {
			t.Errorf("%s: got %d %s", tc.name, st, b)
		}
	}
	if got := nodeCalls(); len(got) != 0 {
		t.Fatalf("bad input reached the runner: %+v", got)
	}
}

func TestNodesAuthzUpgradeAllAndJSON(t *testing.T) {
	r := nodeRig(t)
	tok := r.token("ada", roles.Admin, time.Hour, roles.AllProjects)
	if st, b := r.do(r.client(), "POST", PathNodesUpgrade, tok, `{"all":true,"allow_downgrade":true,"json":true}`); st != 200 {
		t.Fatalf("%d %s", st, b)
	}
	got := nodeCalls()
	if len(got) != 1 || got[0].cmd.Node != "" || !got[0].cmd.JSON || strings.Join(got[0].cmd.Args, " ") != "--all --allow-downgrade" {
		t.Fatalf("ran %+v", got)
	}
}

func TestNodesAuthzFailedCommandIs422(t *testing.T) {
	r := nodeRig(t)
	nodeFake.Lock()
	nodeFake.exit = 1
	nodeFake.Unlock()
	tok := r.token("ada", roles.Admin, time.Hour, roles.AllProjects)
	st, b := r.do(r.client(), "POST", PathNodesPfx+"n1/revoke", tok, "")
	var res NodeResult
	if st != 422 || json.Unmarshal(b, &res) != nil || res.Exit != 1 {
		t.Fatalf("%d %s", st, b)
	}
}

func TestNodesAuthzUnavailableBeforeServe(t *testing.T) {
	r := nodeRig(t)
	nodeFake.Lock()
	nodeFake.on = false
	nodeFake.Unlock()
	tok := r.token("ada", roles.Admin, time.Hour, roles.AllProjects)
	st, b := r.do(r.client(), "GET", PathNodes, tok, "")
	refused(t, st, b, 404, ReasonNotFound)
}

func TestNodesAuthzRotateKeyIDAndContentType(t *testing.T) {
	r := nodeRig(t)
	tok := r.token("ada", roles.Admin, time.Hour, roles.AllProjects)
	if st, b := r.do(r.client(), "POST", PathNodesPfx+"n1/rotate", tok, `{"mode":"node-key","key_id":"k1"}`); st != 200 {
		t.Fatalf("%d %s", st, b)
	}
	got := nodeCalls()
	if len(got) != 1 || strings.Join(got[0].cmd.Args, " ") != "--mode=node-key --key-id=k1" {
		t.Fatalf("ran %+v", got)
	}
	// a form post (the shape a cross-site request has) is refused before the runner
	req, _ := http.NewRequest("POST", r.srv.URL+PathNodesPfx+"n1/revoke", strings.NewReader("a=b"))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := r.client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 400 || len(nodeCalls()) != 1 {
		t.Fatalf("form post = %d, runner calls %d", resp.StatusCode, len(nodeCalls()))
	}
}

func TestNodesAuthzScopedMemberCannotReadNodes(t *testing.T) {
	r := nodeRig(t)
	tok := r.token("eve", roles.Member, time.Hour, "acme/app") // a member of one project
	for _, path := range []string{PathNodes, PathNodesPfx + "n1"} {
		st, b := r.do(r.client(), "GET", path, tok, "")
		refused(t, st, b, 403, ReasonProject)
	}
	if got := nodeCalls(); len(got) != 0 {
		t.Fatalf("ran %+v", got)
	}
}

func TestNodesAuthzReadFailureHidesStoreText(t *testing.T) {
	r := nodeRig(t)
	nodeFake.Lock()
	nodeFake.exit = 2
	nodeFake.Unlock()
	tok := r.token("bob", roles.Member, time.Hour, roles.AllProjects)
	st, b := r.do(r.client(), "GET", PathNodes, tok, "")
	var res NodeResult
	if st != 422 || json.Unmarshal(b, &res) != nil || res.Exit != 2 || strings.Contains(res.Stdout+res.Stderr, "ran") {
		t.Fatalf("%d %s", st, b)
	}
}
