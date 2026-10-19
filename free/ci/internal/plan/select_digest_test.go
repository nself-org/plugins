package plan

// Tests for debt D-1006: the selection digest covers the changed paths, a
// selector's own digest survives, and a release pipeline reaches the selector.

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func digestCfg() model.PipelineConfig {
	lint := job()
	lint.Path = "fast"
	cfg := cfgOf(map[string]model.Job{"lint": lint, "test": job()})
	cfg.Pipelines = map[string]model.PipelineDef{
		"release": {Triggers: []model.Trigger{"push"}, Jobs: []string{"lint", "test"}},
	}
	return cfg
}

// pickLint always selects the lint job, whatever the diff.
func pickLint(SelectionInput) (model.Selection, []string) {
	return model.Selection{Mode: "affected", Reason: "selection.affected"}, []string{"lint"}
}

func TestSelectionDigestCoversChangedPaths(t *testing.T) {
	defer SetSelector(nil)
	SetSelector(pickLint)
	digest := func(changed ...string) string {
		rev := Revision{Branch: "feature/x", BaseKnown: true, Changed: changed}
		p := mustBuild(t, Input{World: linuxWorld(), Config: digestCfg(), Revision: rev})
		if len(p.Jobs) != 1 || p.Jobs[0].Name != "lint" {
			t.Fatalf("jobs = %+v, want lint only", p.Jobs)
		}
		return p.Selection.Digest
	}
	a, b := digest("a.go"), digest("b.go")
	if a == b {
		t.Errorf("two different diffs that pick the same jobs share digest %s", a)
	}
	if a != digest("a.go") {
		t.Error("the digest is not stable for the same diff")
	}
}

func TestSelectorDigestIsKept(t *testing.T) {
	defer SetSelector(nil)
	const own = "feedface"
	SetSelector(func(in SelectionInput) (model.Selection, []string) {
		sel, ids := pickLint(in)
		sel.Digest = own
		return sel, ids
	})
	rev := Revision{Branch: "feature/x", BaseKnown: true, Changed: []string{"a.go"}}
	p := mustBuild(t, Input{World: linuxWorld(), Config: digestCfg(), Revision: rev})
	if p.Selection.Digest != own {
		t.Errorf("digest = %s, want the selector's %s", p.Selection.Digest, own)
	}
}
