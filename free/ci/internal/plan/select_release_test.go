package plan

// Test for debt D-1006: a release pipeline reaches the selector and the
// path-glob default answers full for it.

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestReleasePipelineReachesSelector(t *testing.T) {
	defer SetSelector(nil)
	var seen []bool
	SetSelector(func(in SelectionInput) (model.Selection, []string) {
		seen = append(seen, in.ReleasePipeline)
		return pickLint(in)
	})
	rev := Revision{Branch: "feature/x", BaseKnown: true, Changed: []string{"a.go"}}
	p := mustBuild(t, Input{World: linuxWorld(), Config: digestCfg(), Revision: rev, Pipeline: "release"})
	if len(seen) != 1 || !seen[0] {
		t.Errorf("selector saw ReleasePipeline = %v, want [true]", seen)
	}
	_ = p
	// The path-glob default answers full for a release pipeline.
	SetSelector(nil)
	p = mustBuild(t, Input{World: linuxWorld(), Config: digestCfg(), Revision: rev, Pipeline: "release"})
	if p.Selection.Mode != "full" || p.Selection.Reason != "selection.full_required" {
		t.Errorf("default selector on a release pipeline: %+v", p.Selection)
	}
}
