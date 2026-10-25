package main

// Purpose: end-to-end tests of `nself ci nodes <sub> --coordinator <url>` (P7-NODE-26): the real client,
// the real user API over TLS, the real runner on a real store. A member lists and shows but cannot
// revoke; an admin revokes (state, audit rows chained and verified); add and pair are refused with E664
// local_only before anything is sent; without the flag the command stays local.
// Inputs:  a temp store with one registered node and a user API in front of it. Outputs: pass or fail.
// Constraints: 127.0.0.1 only; keys generated per test.

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/coordclient"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type remoteRig struct {
	t    *testing.T
	st   *store.Store
	svc  *roles.Service
	url  string
	root string
	base int // node audit rows before the test acts
}

func newRemoteRig(t *testing.T) *remoteRig {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	raw, err := os.ReadFile("../internal/model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	c.Identity.ID, c.Identity.Name = "lap1", "lap1"
	if _, err = registry.New(st).Register(context.Background(), c, "operator"); err != nil {
		t.Fatal(err)
	}
	ca, err := pki.New(context.Background(), pki.Options{Store: st, DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := ca.IssueServer("coord-test", []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")}, 0)
	if err != nil {
		t.Fatal(err)
	}
	svc := roles.New(st, &audit.Log{St: st})
	svc.Self = "alice"
	api := clientapi.New(clientapi.Config{Roles: svc, Backend: &clientapi.StoreBackend{Store: st, Home: dir}})
	srv := httptest.NewUnstartedServer(api.Handler())
	srv.Listener = tls.NewListener(srv.Listener, clientapi.TLSConfig(ca, func() (tls.Certificate, error) { return leaf, nil }, pki.NewGuard(st)))
	srv.Start()
	t.Cleanup(srv.Close)
	nodeAdminDeps.Store(&nodeAdminState{store: st, home: dir})
	t.Cleanup(func() { nodeAdminDeps.Store(nil) })
	root := filepath.Join(t.TempDir(), "root.pem")
	if err = os.WriteFile(root, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.RootDER()}), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(coordclient.EnvCA, root)
	r := &remoteRig{t: t, st: st, svc: svc, url: "https://" + srv.Listener.Addr().String(), root: root}
	r.base = r.auditRows("node.") // the registration row of the fixture node
	return r
}

// as makes the next commands run with a token of the given role.
func (r *remoteRig) as(principal string, role roles.Role) {
	r.t.Helper()
	tok, _, err := r.svc.IssueToken(context.Background(), r.svc.Operator("cli:alice"), roles.Request{Principal: principal, Role: role, Projects: []string{roles.AllProjects}, TTL: time.Hour})
	if err != nil {
		r.t.Fatal(err)
	}
	p := filepath.Join(r.t.TempDir(), "token")
	if err = os.WriteFile(p, []byte(tok+"\n"), 0o600); err != nil {
		r.t.Fatal(err)
	}
	r.t.Setenv(coordclient.EnvTokenFile, p)
}

// run dispatches `key args… --coordinator url` with captured output.
func (r *remoteRig) run(key string, args ...string) (code int, handled bool, out, errw string) {
	r.t.Helper()
	var ob, eb bytes.Buffer
	oldO, oldE := coordOut, coordErr
	coordOut, coordErr = &ob, &eb
	defer func() { coordOut, coordErr = oldO, oldE }()
	code, handled = nodesRemoteDispatch(key, append(args, "--coordinator", r.url))
	return code, handled, ob.String(), eb.String()
}

func (r *remoteRig) state() string {
	r.t.Helper()
	n, err := registry.New(r.st).Get(context.Background(), "lap1")
	if err != nil {
		r.t.Fatal(err)
	}
	return n.Record.State
}

func (r *remoteRig) auditRows(prefix string) (n int) {
	r.t.Helper()
	es, err := r.st.AuditEntries(context.Background(), 0, 10000)
	if err != nil {
		r.t.Fatal(err)
	}
	for _, e := range es {
		if strings.HasPrefix(e.Action, prefix) {
			n++
		}
	}
	return n
}

func TestNodesRemoteMemberReadsButCannotRevoke(t *testing.T) {
	r := newRemoteRig(t)
	r.as("bob", roles.Member)
	if code, handled, out, errw := r.run("nodes list", "--json"); !handled || code != 0 || !strings.Contains(out, "lap1") {
		t.Fatalf("list = %d %v %q %q", code, handled, out, errw)
	}
	if code, _, out, errw := r.run("nodes show", "lap1", "--json"); code != 0 || !strings.Contains(out, "lap1") {
		t.Fatalf("show = %d %q %q", code, out, errw)
	}
	before := r.state()
	for _, sub := range []string{"revoke", "remove", "drain", "enable"} {
		code, _, _, errw := r.run("nodes "+sub, "lap1")
		if code != 1 || !strings.Contains(errw, "E664") || !strings.Contains(errw, "(role)") {
			t.Fatalf("member %s = %d %q, want exit 1 with E664 reason role", sub, code, errw)
		}
	}
	if code, _, _, errw := r.run("nodes set", "lap1", "--reserve-cpu", "1"); code != 1 || !strings.Contains(errw, "(role)") {
		t.Fatalf("member set = %d %q", code, errw)
	}
	if got := r.state(); got != before || r.auditRows("node.") != r.base {
		t.Fatalf("a refused member changed the node: state %s -> %s, %d node audit rows, want %d", before, got, r.auditRows("node."), r.base)
	}
}

func TestNodesRemoteAdminRevokesWithChainedAudit(t *testing.T) {
	r := newRemoteRig(t)
	r.as("ada", roles.Admin)
	if code, _, out, errw := r.run("nodes drain", "lap1", "--json"); code != 0 || !strings.Contains(out, "drained") {
		t.Fatalf("drain = %d %q %q", code, out, errw)
	}
	if code, _, out, errw := r.run("nodes set", "lap1", "--reserve-cpu", "50%", "--json"); code != 0 || !strings.Contains(out, "settings") {
		t.Fatalf("set = %d %q %q", code, out, errw)
	}
	if code, _, _, errw := r.run("nodes enable", "lap1"); code != 0 {
		t.Fatalf("enable = %d %q", code, errw)
	}
	code, _, out, errw := r.run("nodes revoke", "lap1", "--json")
	if code != 0 || !strings.Contains(out, `"node"`) {
		t.Fatalf("revoke = %d %q %q", code, out, errw)
	}
	if got := r.state(); got != "revoked" {
		t.Fatalf("state after revoke = %s", got)
	}
	// the in-process revoke runs the registered revokers (the pairing deny list) and session closers (the
	// agent listener's live streams), exactly as the local command does
	var env struct {
		Data struct {
			Closed int `json:"sessions_closed"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || env.Data.Closed < 1 {
		t.Fatalf("revoke closed no live sessions: %s (%v)", out, err)
	}
	// each mutation: one decision row (authorize, admin) and exactly one node row, both under the caller
	byAction := map[string]int{}
	allEs, _ := r.st.AuditEntries(context.Background(), 0, 10000)
	for _, e := range allEs {
		if strings.HasPrefix(e.Action, "node.") && strings.Contains(e.Actor, "ada") {
			byAction[e.Action]++
		}
	}
	if byAction["node.state"] != 2 || byAction["node.settings"] != 1 || byAction["node.revoke"] != 1 || len(byAction) != 3 {
		t.Fatalf("node audit rows by action under the caller: %v (want drain+enable state 2, settings 1, revoke 1)", byAction)
	}
	if n := r.auditRows("node.revoke"); n != 1 {
		t.Fatalf("revoke wrote %d node.revoke rows, want 1", n)
	}
	es, _ := r.st.AuditEntries(context.Background(), 0, 10000)
	found := false
	for _, e := range es {
		if strings.HasPrefix(e.Action, "node.") && strings.Contains(e.Actor, "ada") {
			found = true
		}
		if strings.Contains(e.Detail, "eyJ") {
			t.Fatalf("audit row carries a token-shaped value: %+v", e)
		}
	}
	if !found {
		t.Fatalf("no node audit row names the caller: %+v", es)
	}
	log := &audit.Log{St: r.st}
	if _, err := log.Seal(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rep, err := log.Verify(context.Background(), time.Hour); err != nil || !rep.OK() || rep.Sealed == 0 {
		t.Fatalf("chain verify = %+v %v", rep, err)
	}
	// the node is gone after remove; its audit rows stay
	if code, _, _, errw := r.run("nodes remove", "lap1"); code != 0 {
		t.Fatalf("remove = %d %q", code, errw)
	}
	if r.auditRows("node.revoke") != 1 || r.auditRows("node.remove") < 1 {
		t.Fatalf("audit rows after remove: revoke %d remove %d", r.auditRows("node.revoke"), r.auditRows("node.remove"))
	}
}

func TestNodesRemoteUnknownNodeKeepsExitClass(t *testing.T) {
	r := newRemoteRig(t)
	r.as("ada", roles.Admin)
	code, _, _, errw := r.run("nodes revoke", "nosuch")
	if code != 1 || !strings.Contains(errw, "E650") {
		t.Fatalf("unknown node = %d %q, want exit 1 E650", code, errw)
	}
}

func TestNodesRemoteAddAndPairAreLocalOnly(t *testing.T) {
	r := newRemoteRig(t)
	r.as("ada", roles.Admin)
	for _, key := range []string{"nodes add", "nodes pair", "nodes test"} {
		code, handled, _, errw := r.run(key, "x")
		if !handled || code != 1 || !strings.Contains(errw, "local_only") || !strings.Contains(errw, "coordinator host") {
			t.Fatalf("%s = %d %v %q, want exit 1 E664 local_only", key, code, handled, errw)
		}
	}
	if r.auditRows("node.") != r.base {
		t.Fatal("a refused add or pair touched the audit log")
	}
}

func TestNodesRemoteStaysLocalWithoutTheFlag(t *testing.T) {
	for _, key := range []string{"nodes list", "nodes revoke", "nodes set", "nodes upgrade", "nodes discover", "nodes secrets get"} {
		if _, handled := nodesRemoteDispatch(key, []string{"lap1", "--json"}); handled {
			t.Fatalf("%s without --coordinator was taken over", key)
		}
	}
	if _, handled := nodesRemoteDispatch("nodes discover", []string{"--coordinator", "https://x"}); handled {
		t.Fatal("a command the API does not know was taken over")
	}
}

func TestNodesRemoteBadFlagsNeverSend(t *testing.T) {
	r := newRemoteRig(t)
	r.as("ada", roles.Admin)
	for _, args := range [][]string{{"lap1", "--bogus"}, {}, {"a", "b"}, {"lap1", "--agent-binary", "/x"}} {
		if code, _, _, errw := r.run("nodes revoke", args...); code != 1 || !strings.Contains(errw, "bad_request") {
			t.Fatalf("revoke %v = %d %q", args, code, errw)
		}
	}
	if r.state() == "revoked" {
		t.Fatal("a bad request revoked the node")
	}
	if _, _, _, errw := nodesRemoteDispatchErr("nodes list", []string{"--coordinator"}); !strings.Contains(errw, "needs a URL") {
		t.Fatalf("bare --coordinator: %q", errw)
	}
}

func nodesRemoteDispatchErr(key string, args []string) (code int, handled bool, out, errw string) {
	var ob, eb bytes.Buffer
	oldO, oldE := coordOut, coordErr
	coordOut, coordErr = &ob, &eb
	defer func() { coordOut, coordErr = oldO, oldE }()
	code, handled = nodesRemoteDispatch(key, args)
	return code, handled, ob.String(), eb.String()
}

func TestNodesRemoteRequestBuildsRotateAndUpgradeFlags(t *testing.T) {
	id, q, err := nodesRemoteRequest("rotate", []string{"lap1", "--node-key", "--key-id", "k1"})
	if err != nil || id != "lap1" || q.Mode != "node-key" || q.KeyID != "k1" {
		t.Fatalf("rotate = %q %+v %v", id, q, err)
	}
	if _, q, err = nodesRemoteRequest("upgrade", []string{"--all", "--version", "1.5.0", "--allow-downgrade"}); err != nil || !q.All || q.Version != "1.5.0" || !q.AllowDowngrade || q.Node != "" {
		t.Fatalf("upgrade --all = %+v %v", q, err)
	}
	if id, q, err = nodesRemoteRequest("upgrade", []string{"Lap1"}); err != nil || id != "" || q.Node != "lap1" {
		t.Fatalf("upgrade node = %q %+v %v", id, q, err)
	}
	if _, _, err = nodesRemoteRequest("upgrade", []string{"--all", "lap1"}); err == nil {
		t.Fatal("upgrade with a node and --all was accepted")
	}
}

func TestNodesRemoteOutputOverLimitIsAnError(t *testing.T) {
	var w capWriter
	if _, err := w.Write(make([]byte, nodesOutCap+1)); err != nil || !w.over || w.b.Len() != nodesOutCap {
		t.Fatalf("over=%v len=%d err=%v", w.over, w.b.Len(), err)
	}
	var ok capWriter
	_, _ = ok.Write(make([]byte, nodesOutCap))
	if ok.over {
		t.Fatal("output of exactly the limit was flagged")
	}
}

func TestNodesRemoteUpgradeClaimIsPerNodeAndRefusesNotQueues(t *testing.T) {
	up := func(node string, args ...string) clientapi.NodeCommand {
		return clientapi.NodeCommand{Verb: "upgrade", Node: node, Args: args}
	}
	relA, err := claimNode(up("a"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = claimNode(up("a")); clientapi.ReasonOf(err) != clientapi.ReasonConflict {
		t.Fatalf("second upgrade of a: %v", err)
	}
	if _, err = claimNode(clientapi.NodeCommand{Verb: "rotate", Node: "a", Args: []string{"--mode=upgrade"}}); clientapi.ReasonOf(err) != clientapi.ReasonConflict {
		t.Fatalf("rotate in upgrade mode on a busy node: %v", err)
	}
	if rel, err := claimNode(clientapi.NodeCommand{Verb: "rotate", Node: "a", Args: []string{"--cert"}}); err != nil {
		t.Fatalf("an emergency cert rotation was blocked: %v", err)
	} else {
		rel()
	}
	relB, err := claimNode(up("b"))
	if err != nil {
		t.Fatalf("another node: %v", err)
	}
	if _, err = claimNode(up("")); clientapi.ReasonOf(err) != clientapi.ReasonConflict {
		t.Fatalf("upgrade --all beside single upgrades: %v", err)
	}
	relA()
	relB()
	relAll, err := claimNode(up(""))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = claimNode(up("c")); clientapi.ReasonOf(err) != clientapi.ReasonConflict {
		t.Fatalf("upgrade during upgrade --all: %v", err)
	}
	relAll()
	if rel, err := claimNode(up("c")); err != nil {
		t.Fatalf("after release: %v", err)
	} else {
		rel()
	}
}

// TestRoutedNodesFlagSetsListCoordinator: help and the generated flag reference come from each subcommand's
// own flag set, so every routed `nodes` command must declare --coordinator there (D-1409).
func TestRoutedNodesFlagSetsListCoordinator(t *testing.T) {
	for sub := range nodesRoutedSubs {
		key := "nodes " + sub
		e, ok := v15Subcommands[key]
		if !ok || e.Flags == nil {
			t.Errorf("%s: no V15 entry with a flag set", key)
			continue
		}
		if e.Flags().Lookup("coordinator") == nil {
			t.Errorf("%s: its flag set does not declare --coordinator", key)
		}
	}
}
