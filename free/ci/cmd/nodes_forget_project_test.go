package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// seedRoot caches one exclusion entry under root, as a refresh of a project that deployed would.
func seedRoot(t *testing.T, home, root string) {
	t.Helper()
	st, err := deployguard.OpenStore(filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := context.Background()
	if err = st.ExclusionProjectSeen(ctx, root); err != nil {
		t.Fatal(err)
	}
	if err = st.ExclusionReplace(ctx, root, []store.ExclusionEntry{{Root: root, Kind: "host", Value: deployguard.FixtureHostV4, Source: "inventory"}}); err != nil {
		t.Fatal(err)
	}
}

func knownRoots(t *testing.T, home string) []string {
	t.Helper()
	st, err := deployguard.OpenStore(filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	roots, err := st.ExclusionProjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return roots
}

func TestForgetProjectIsANodesSubcommand(t *testing.T) {
	e, ok := v15Subcommands["nodes forget-project"]
	if !ok || !e.V15Only || e.Handler == nil || e.Flags == nil {
		t.Fatalf("nodes forget-project is not registered: %+v", e)
	}
}

// A deleted project root that never read OK stays in the cache and blocks `nodes add`; the operator must be
// able to drop it from the CLI, with the audit row Guard.ForgetProject writes.
func TestForgetProjectDropsAGoneRootAndItsEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	gone := filepath.Join(t.TempDir(), "deleted-project")
	keep := t.TempDir()
	seedRoot(t, home, gone)
	seedRoot(t, home, keep)
	var out, errOut bytes.Buffer
	if code := runNodesForgetProject([]string{gone}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s%s", code, out.String(), errOut.String())
	}
	if roots := knownRoots(t, home); len(roots) != 1 || roots[0] != keep {
		t.Fatalf("roots after forget: %v", roots)
	}
	st, err := deployguard.OpenStore(filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	rows, err := st.ExclusionEntries(context.Background())
	if err != nil || len(rows) != 1 || rows[0].Root != keep {
		t.Fatalf("entries after forget: %v %v", rows, err)
	}
	if !strings.Contains(out.String(), gone) {
		t.Fatalf("output does not name the root: %q", out.String())
	}
}

func TestForgetProjectRefusesBadInput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	known := filepath.Join(t.TempDir(), "p")
	seedRoot(t, home, known)
	for name, args := range map[string][]string{
		"no root":       {},
		"two roots":     {known, known},
		"relative root": {"p"},
		"unknown root":  {filepath.Join(t.TempDir(), "never-seen")},
	} {
		var out, errOut bytes.Buffer
		if code := runNodesForgetProject(args, &out, &errOut); code != 1 {
			t.Errorf("%s: exit %d, want 1 (%s%s)", name, code, out.String(), errOut.String())
		}
	}
	if roots := knownRoots(t, home); len(roots) != 1 {
		t.Fatalf("a refused call changed the cache: %v", roots)
	}
	if _, err := os.Stat(filepath.Join(home, "state.db")); err != nil {
		t.Fatal(err)
	}
}
