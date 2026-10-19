package affected

// Purpose: D-1007 regression. The directory-change check of the selector
// must not rescan every package for every changed path.

import (
	"fmt"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/plan"
)

func TestSelectScalesWithChangedPathsAndPackages(t *testing.T) {
	const pkgs, files = 5000, 20000
	g := newGraph()
	for i := 0; i < pkgs; i++ {
		g.addNode(fmt.Sprintf("mod%d/pkg%d", i%50, i), "go")
	}
	changed := make([]string, 0, files)
	for i := 0; i < files; i++ {
		changed = append(changed, fmt.Sprintf("mod%d/pkg%d/file%d.go", i%50, i%pkgs, i))
	}
	in := plan.SelectionInput{Changed: changed, BaseKnown: true,
		Jobs: []plan.JobRef{{ID: "root.test", Workdir: "."}}}

	start := time.Now()
	sel, _ := selectWith(g, true, in)
	elapsed := time.Since(start)

	if sel.Mode != "affected" {
		t.Fatalf("mode = %q (reason %q), want affected", sel.Mode, sel.Reason)
	}
	// 100 million prefix checks took seconds; one pass over the nodes is milliseconds.
	if elapsed > 750*time.Millisecond {
		t.Fatalf("selection of %d paths over %d packages took %v, want < 750ms", files, pkgs, elapsed)
	}
}

func TestHoldsNodesOnlyForDirectoriesAboveNodes(t *testing.T) {
	g := newGraph()
	g.addNode("a/b/c", "go")
	g.opaque["x/y"] = "cargo"
	holder := g.nodeAncestors()
	for p, want := range map[string]bool{"a": true, "a/b": true, "a/b/c": false, "a/b/c/f.go": false,
		"x": true, "x/y": false, "z": false, ".": false} {
		if holder[p] != want {
			t.Errorf("holder[%q] = %v, want %v", p, holder[p], want)
		}
	}
}
