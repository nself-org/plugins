package labels

// Purpose: tests of the label edit and the project authorization edit (P7-NODE-30) against a real
// SQLite store and registry: results, rejected edits that change nothing, the audit trail, the TEAM
// refusals and the `--all` expansion at call time.
// Inputs:  the laptop capability fixture registered under several node ids.
// Outputs: pass or fail.
// Constraints: no network, no processes.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func newEnv(t *testing.T, nodes ...string) Env {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	b, err := os.ReadFile("../../model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	reg := registry.New(st)
	for _, id := range nodes {
		var c model.Capability
		if err = json.Unmarshal(b, &c); err != nil {
			t.Fatal(err)
		}
		c.Identity.ID, c.Identity.Name = id, id
		if _, err = reg.Register(context.Background(), c, "operator"); err != nil {
			t.Fatal(err)
		}
	}
	return Env{Store: st, Registry: reg, Actor: "cli:test", ResolveDir: func(_ context.Context, dir string) (string, bool) {
		if strings.HasSuffix(dir, "project-b") {
			return "local:bbb", true
		}
		return "", false
	}}
}

func TestEditSetsRemovesAndAudits(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, "mini1")
	got, err := e.Edit(ctx, "mini1", []string{"xcode", "arch=arm64"}, nil)
	if err != nil || !reflect.DeepEqual(Strings(got), []string{"arch=arm64", "xcode"}) {
		t.Fatalf("set: %v %v", got, err)
	}
	if got, err = e.Edit(ctx, "mini1", nil, []string{"xcode"}); err != nil || !reflect.DeepEqual(Strings(got), []string{"arch=arm64"}) {
		t.Fatalf("remove: %v %v", got, err)
	}
	audit, err := e.Store.NodeLabelAudit(ctx, "mini1")
	if err != nil || len(audit) != 3 {
		t.Fatalf("audit rows %d %v, want 3 (xcode set, arch set, xcode remove)", len(audit), err)
	}
}

func TestEditRejectsAndChangesNothing(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, "mini1")
	if _, err := e.Edit(ctx, "mini1", []string{"keep"}, nil); err != nil {
		t.Fatal(err)
	}
	many := make([]string, 0, MaxLabels)
	for i := 0; i < MaxLabels; i++ {
		many = append(many, "k"+strings.Repeat("x", i))
	}
	for name, c := range map[string]struct {
		add    []string
		reason string
	}{"bad key": {[]string{"Bad Key"}, ReasonSyntax}, "reserved": {[]string{"nself.fabric"}, ReasonReserved}, "33rd": {many, ReasonTooMany}} {
		_, err := e.Edit(ctx, "mini1", c.add, nil)
		if ReasonOf(err) != c.reason {
			t.Fatalf("%s: %v, want reason %s", name, err, c.reason)
		}
	}
	got, _ := e.Store.NodeLabels(ctx, "mini1")
	audit, _ := e.Store.NodeLabelAudit(ctx, "mini1")
	if len(got) != 1 || len(audit) != 1 {
		t.Fatalf("a rejected edit changed state: %v, %d audit rows", got, len(audit))
	}
	if _, err := e.Edit(ctx, "ghost", []string{"x"}, nil); err == nil {
		t.Fatal("unregistered node accepted")
	}
}

func TestLocalAlias(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	if _, err := e.Edit(ctx, LocalNode, []string{"gpu"}, nil); err != nil {
		t.Fatal(err)
	}
	got, err := e.Store.NodeLabels(ctx, LocalNode)
	if err != nil || !reflect.DeepEqual(Strings(got), []string{"gpu"}) {
		t.Fatalf("local labels %v %v", got, err)
	}
	e.Team = true
	if _, err = e.Edit(ctx, LocalNode, []string{"gpu"}, nil); err == nil {
		t.Fatal("local alias accepted on a coordinator")
	}
}

func TestProjectsAddRemoveList(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, "vps-a")
	added, err := e.AddProjects(ctx, "vps-a", []string{"proj-a", "../project-b", "proj-a"}, false)
	if err != nil || !reflect.DeepEqual(added, []string{"proj-a", "local:bbb"}) {
		t.Fatalf("add: %v %v", added, err)
	}
	if added, err = e.AddProjects(ctx, "vps-a", []string{"proj-a"}, false); err != nil || len(added) != 0 {
		t.Fatalf("re-add: %v %v", added, err)
	}
	removed, err := e.RemoveProjects(ctx, "vps-a", []string{"proj-a"}, false)
	if err != nil || !reflect.DeepEqual(removed, []string{"proj-a"}) {
		t.Fatalf("remove: %v %v", removed, err)
	}
	if got, _ := e.Projects(ctx, "vps-a"); !reflect.DeepEqual(got, []string{"local:bbb"}) {
		t.Fatalf("list: %v", got)
	}
	if _, err = e.AddProjects(ctx, "vps-a", []string{"./nowhere"}, false); ReasonOf(err) != ReasonProjectUnknown {
		t.Fatalf("unresolvable directory: %v", err)
	}
	if _, err = e.AddProjects(ctx, "vps-a", []string{"bad id"}, false); ReasonOf(err) != ReasonProjectUnknown {
		t.Fatalf("bad id: %v", err)
	}
}

func TestProjectsAllIsExplicitAndPersonalOnly(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, "vps-a")
	seed := func(project string) {
		err := e.Store.CreatePipeline(ctx, store.PipelineRow{ID: "pl-" + project, Project: project, Revision: "r", Trigger: "push", SourceTrust: "owner",
			PrivacyZone: "local-only", PolicyDigest: "d", InputDigest: "i", SelectionMode: "m", SelectionReason: "r", SelectionDigest: "s", Status: "queued"})
		if err != nil {
			t.Fatal(err)
		}
	}
	seed("proj-a")
	seed("proj-b")
	added, err := e.AddProjects(ctx, "vps-a", nil, true)
	if err != nil || !reflect.DeepEqual(added, []string{"proj-a", "proj-b"}) {
		t.Fatalf("--all: %v %v", added, err)
	}
	seed("proj-c")
	if got, _ := e.Projects(ctx, "vps-a"); !reflect.DeepEqual(got, []string{"proj-a", "proj-b"}) {
		t.Fatalf("a project seen later was authorized: %v", got)
	}
	team := e
	team.Team = true
	if _, err = team.AddProjects(ctx, "vps-a", nil, true); ReasonOf(err) != ReasonTeamWildcard {
		t.Fatalf("team --all: %v", err)
	}
	if _, err = team.AddProjects(ctx, "vps-a", []string{"./x"}, false); ReasonOf(err) != ReasonProjectUnknown {
		t.Fatalf("team directory: %v", err)
	}
}
