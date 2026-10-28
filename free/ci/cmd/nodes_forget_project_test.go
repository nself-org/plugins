package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestNodesForgetProjectRemovesDeletedUnverifiedRoot(t *testing.T) {
	entry, ok := v15Subcommands["nodes forget-project"]
	if !ok {
		t.Fatal("nodes forget-project is not registered")
	}
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	ctx := context.Background()
	st, err := deployguard.OpenStore(filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	deleted := filepath.Join(home, "deleted") // deliberately never created or successfully read
	keep := filepath.Join(home, "keep")
	for _, root := range []string{deleted, keep} {
		if err := st.ExclusionProjectSeen(ctx, root); err != nil {
			t.Fatal(err)
		}
		if err := st.ExclusionFail(ctx, root, "missing inventory"); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.ExclusionReplace(ctx, keep, []store.ExclusionEntry{{Root: keep, Kind: "host", Value: "203.0.113.10", Source: "inventory"}}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{nil, {"relative"}, {deployguard.NeverListRoot}, {deleted, keep}, {"--coordinator", "https://example.invalid", deleted}} {
		if code := entry.Handler(args); code != 1 {
			t.Fatalf("invalid args %v: exit %d", args, code)
		}
	}
	if code := entry.Handler([]string{deleted}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	roots, err := st.ExclusionProjects(ctx)
	if err != nil || len(roots) != 1 || roots[0] != keep {
		t.Fatalf("roots %v: %v", roots, err)
	}
	states, err := st.ExclusionStates(ctx)
	if err != nil || len(states) != 1 || states[0].Root != keep {
		t.Fatalf("states %v: %v", states, err)
	}
	rows, err := st.ExclusionEntries(ctx)
	if err != nil || len(rows) != 1 || rows[0].Root != keep {
		t.Fatalf("entries %v: %v", rows, err)
	}
	audits, err := st.AuditEntries(ctx, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(audits) != 1 || audits[0].Action != "exclusion.forget" || audits[0].Target != deleted || audits[0].Actor != adminActor() {
		t.Fatalf("expected exactly one operator audit: %+v", audits)
	}
}
