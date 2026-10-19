package store

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func putFixtureNode(t *testing.T, s *Store) string {
	t.Helper()
	fixture, err := os.ReadFile("../model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(fixture, &c); err != nil {
		t.Fatal(err)
	}
	n := NodeRecord{ID: c.Identity.ID, Name: c.Identity.Name, State: "offline", Capability: fixture, Lifecycle: []byte(`{}`), Trust: []byte(`{}`), DeployHost: []byte(`{}`)}
	if err = s.NodePut(context.Background(), n, "operator"); err != nil {
		t.Fatal(err)
	}
	return c.Identity.ID
}

func TestExclusionReplaceFailKeepsEntries(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	if err := s.ExclusionProjectSeen(ctx, "/p/a"); err != nil {
		t.Fatal(err)
	}
	a := []ExclusionEntry{{"/p/a", "host", "203.0.113.10", "inventory"}, {"/p/a", "fingerprint", "SHA256:x", "inventory"}}
	if err := s.ExclusionReplace(ctx, "/p/a", a); err != nil {
		t.Fatal(err)
	}
	if err := s.ExclusionReplace(ctx, "/p/b", []ExclusionEntry{{"/p/b", "host", "203.0.113.11", "inventory"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.ExclusionFail(ctx, "/p/a", "nself exited 1"); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.ExclusionEntries(ctx)
	if len(rows) != 3 {
		t.Fatalf("a failed refresh changed the entries: %+v", rows)
	}
	st, _ := s.ExclusionStates(ctx)
	if len(st) != 2 || st[0].Root != "/p/a" || st[0].OK || st[0].Error == "" || st[0].LastOKAt == 0 || !st[1].OK {
		t.Fatalf("states %+v", st)
	}
	// A later success replaces that root's rows only and clears the failure.
	if err := s.ExclusionReplace(ctx, "/p/a", a[:1]); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.ExclusionEntries(ctx)
	st, _ = s.ExclusionStates(ctx)
	if len(rows) != 2 || !st[0].OK || st[0].Error != "" {
		t.Fatalf("after success: %+v %+v", rows, st)
	}
	roots, _ := s.ExclusionProjects(ctx)
	if len(roots) != 1 || roots[0] != "/p/a" {
		t.Fatalf("only a seen root is a project: %v", roots)
	}
	for _, bad := range []func() error{
		func() error { return s.ExclusionProjectSeen(ctx, "") },
		func() error { return s.ExclusionReplace(ctx, "", nil) },
		func() error { return s.ExclusionFail(ctx, "", "x") },
	} {
		if err := bad(); !codeIs(err, "E607") {
			t.Fatalf("empty root accepted: %v", err)
		}
	}
	if err := s.ExclusionReplace(ctx, "/p/c", []ExclusionEntry{{"/p/c", "bogus", "x", "inventory"}}); err == nil {
		t.Fatal("unknown kind accepted by the table check")
	}
}

func TestNodeSetDeployHostWritesColumnDocumentAndOneAuditRow(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	id := putFixtureNode(t, s)
	src := "inventory"
	want := model.CapabilityDeployHost{Matched: true, Source: &src, Override: true}
	before, _ := s.NodeGet(ctx, id)
	if err := s.NodeSetDeployHost(ctx, id, want, "cli:test", "matched host"); err != nil {
		t.Fatal(err)
	}
	n, _ := s.NodeGet(ctx, id)
	var c model.Capability
	if err := json.Unmarshal(n.Capability, &c); err != nil {
		t.Fatal(err)
	}
	if !c.DeployHost.Matched || !c.DeployHost.Override || c.DeployHost.Source == nil || *c.DeployHost.Source != "inventory" ||
		!strings.Contains(string(n.DeployHost), `"matched":true`) || n.Version != before.Version+1 || n.Digest == before.Digest {
		t.Fatalf("node %+v doc %+v", n, c.DeployHost)
	}
	audits := countAudit(t, s, "node.deploy_host")
	if audits != 1 {
		t.Fatalf("audit rows = %d", audits)
	}
	// The same fact again writes nothing: no version bump, no second audit row.
	if err := s.NodeSetDeployHost(ctx, id, want, "cli:test", "again"); err != nil {
		t.Fatal(err)
	}
	again, _ := s.NodeGet(ctx, id)
	if again.Version != n.Version || countAudit(t, s, "node.deploy_host") != 1 {
		t.Fatal("an unchanged fact wrote")
	}
	// Clearing it is a change.
	if err := s.NodeSetDeployHost(ctx, id, model.CapabilityDeployHost{}, "cli:test", "cleared"); err != nil {
		t.Fatal(err)
	}
	if countAudit(t, s, "node.deploy_host") != 2 {
		t.Fatal("clearing did not audit")
	}
	if err := s.NodeSetDeployHost(ctx, "ghost", want, "cli:test", "x"); !codeIs(err, "E650") {
		t.Fatalf("missing node: %v", err)
	}
}
