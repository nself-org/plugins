package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// stubNself makes the hook's guard read the fixture inventory (host 203.0.113.10) instead of running nself.
func stubNself(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	root := t.TempDir()
	t.Setenv("NSELF_CI_REPO", root)
	if err := os.Mkdir(filepath.Join(root, ".nself"), 0o700); err != nil {
		t.Fatal(err)
	}
	run := &deployguard.FixtureRunner{HelpSide: "read", HostsByRoot: map[string][]string{root: {deployguard.FixtureHostV4}}}
	old := newGuard
	newGuard = func(st *store.Store, h, actor string) *deployguard.Guard {
		g := old(st, h, actor)
		g.Reader = deployguard.Reader{Run: run.Run}
		g.Resolver = deployguard.Resolver{}
		return g
	}
	t.Cleanup(func() { newGuard = old })
	return home
}

func draft(t *testing.T, id, host string) *model.Capability {
	t.Helper()
	b, err := os.ReadFile("../internal/model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	c.Identity.ID, c.Identity.Name = id, id
	c.Identity.SSH, c.Identity.HostKeyFingerprints = &model.CapabilitySSH{Alias: host, Hostname: host, Port: 22, User: "ci"}, nil
	c.Identity.MachineID = model.Fact[string]{Source: "self", ObservedAt: time.Now().UTC(), Confidence: "unknown"}
	return &c
}

func addFlagSet(allow bool) *flag.FlagSet {
	fs := lifecycle.FlagSet("add")
	if allow {
		_ = fs.Set(allowDeployHostFlag, "true")
	}
	return fs
}

func TestAllowDeployHostIsAnAddFlag(t *testing.T) {
	f := lifecycle.FlagSet("add").Lookup(allowDeployHostFlag)
	if f == nil || f.DefValue != "false" {
		t.Fatalf("flag %+v", f)
	}
	if lifecycle.FlagSet("test").Lookup(allowDeployHostFlag) != nil {
		t.Fatal("the flag leaked to other subcommands")
	}
}

func TestAddHookMarksADeployHostIneligibleAndRefusesAnUnconfirmedOverride(t *testing.T) {
	stubNself(t)
	ctx := context.Background()
	c := draft(t, "box", deployguard.FixtureHostV4)
	e := lifecycle.HookEvent{Point: lifecycle.PointAdd, NodeID: "box", Dest: "ci@" + deployguard.FixtureHostV4, Capability: c, Flags: addFlagSet(false)}
	if err := deployHostHook(ctx, lifecycle.PointAdd, e); err != nil {
		t.Fatal(err)
	}
	if !c.DeployHost.Matched || c.DeployHost.Override || c.DeployHost.Source == nil || *c.DeployHost.Source != "inventory" {
		t.Fatalf("draft %+v", c.DeployHost)
	}
	// --allow-deploy-host without a terminal: refused, nothing marked as overridden.
	c2 := draft(t, "box2", deployguard.FixtureHostV4)
	e2 := lifecycle.HookEvent{Point: lifecycle.PointAdd, NodeID: "box2", Dest: deployguard.FixtureHostV4, Capability: c2, Flags: addFlagSet(true)}
	err := deployHostHook(ctx, lifecycle.PointAdd, e2)
	if err == nil || deployguard.ReasonOf(err) != deployguard.ReasonNoTTY || c2.DeployHost.Override {
		t.Fatalf("err %v draft %+v", err, c2.DeployHost)
	}
	// An unlisted host passes untouched, flag or not.
	c3 := draft(t, "fine", deployguard.FixtureOther)
	if err = deployHostHook(ctx, lifecycle.PointAdd, lifecycle.HookEvent{NodeID: "fine", Dest: deployguard.FixtureOther, Capability: c3, Flags: addFlagSet(true)}); err != nil || c3.DeployHost.Matched {
		t.Fatalf("unlisted: %v %+v", err, c3.DeployHost)
	}
}

func putNode(t *testing.T, home string, c *model.Capability) {
	t.Helper()
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	doc, _ := json.Marshal(c)
	n := store.NodeRecord{ID: c.Identity.ID, Name: c.Identity.Name, State: "offline", Capability: doc, Lifecycle: []byte(`{}`), Trust: []byte(`{}`), DeployHost: []byte(`{}`)}
	if err = st.NodePut(context.Background(), n, "operator"); err != nil {
		t.Fatal(err)
	}
}

func nodeDeployHost(t *testing.T, home, id string) model.CapabilityDeployHost {
	t.Helper()
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	n, err := st.NodeGet(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(n.Capability, &c); err != nil {
		t.Fatal(err)
	}
	return c.DeployHost
}

func TestTestHookReevaluatesRegisteredNodes(t *testing.T) {
	home := stubNself(t)
	putNode(t, home, draft(t, "n1", deployguard.FixtureHostV4))
	putNode(t, home, draft(t, "n2", deployguard.FixtureOther))
	if err := deployHostHook(context.Background(), lifecycle.PointTest, lifecycle.HookEvent{NodeID: "n1"}); err != nil {
		t.Fatal(err)
	}
	if !nodeDeployHost(t, home, "n1").Matched || nodeDeployHost(t, home, "n2").Matched {
		t.Fatal("test hook did not set deploy_host.matched on the listed node only")
	}
}

func TestPairHookRefusesAnUnregisteredDeployHostAndMarksARegisteredOne(t *testing.T) {
	home := stubNself(t)
	ctx := context.Background()
	e := lifecycle.HookEvent{Point: lifecycle.PointPair, NodeID: "ghost", Dest: deployguard.FixtureHostV4}
	if err := deployHostHook(ctx, lifecycle.PointPair, e); err == nil || !strings.Contains(err.Error(), "cannot be paired") {
		t.Fatalf("pair of an unregistered deploy host: %v", err)
	}
	putNode(t, home, draft(t, "paired", deployguard.FixtureHostV4))
	if err := deployHostHook(ctx, lifecycle.PointPair, lifecycle.HookEvent{NodeID: "paired", Dest: deployguard.FixtureHostV4}); err != nil {
		t.Fatal(err)
	}
	if !nodeDeployHost(t, home, "paired").Matched {
		t.Fatal("registered node not marked at pair")
	}
	if err := deployHostHook(ctx, lifecycle.PointPair, lifecycle.HookEvent{NodeID: "quiet", Dest: deployguard.FixtureOther}); err != nil {
		t.Fatalf("unlisted pair: %v", err)
	}
}

func TestProjectRootAndHelpers(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NSELF_CI_REPO", sub)
	if projectRoot() != root {
		t.Fatalf("root %q want %q", projectRoot(), root)
	}
	if flagBool(nil, "x") || flagBool(flag.NewFlagSet("x", flag.ContinueOnError), "x") {
		t.Fatal("absent flag read as true")
	}
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	fs.String("s", "", "")
	fs.Bool("b", false, "")
	_ = fs.Set("b", "true")
	if !flagBool(fs, "b") || flagBool(fs, "s") {
		t.Fatal("flagBool")
	}
	if !strings.HasPrefix(adminActor(), "cli:") {
		t.Fatal(adminActor())
	}
	if f, err := os.Open(os.DevNull); err == nil {
		if isTTY(f) {
			t.Fatal("/dev/null counted as a terminal")
		}
		_ = f.Close()
	}
}
