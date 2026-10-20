package pipeline_test

// Purpose: TestTemplateGolden of P7-HOST-06 and the checks that tie the provider to the template: the
// rendering the coordinator compares a pipeline's include file with is the rendering `providers gitlab
// init` writes, and it is byte-stable.
// Inputs:  templates/gitlab (renderer, pins) and its golden file.
// Outputs: pass or fail.
// Constraints: no I/O beyond reading the golden; the golden path is relative to this package.

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/pipeline"
	gitlabtmpl "github.com/nself-org/plugins/free/ci/templates/gitlab"
)

const goldenPath = "../../../../templates/gitlab/testdata/include.golden.yml"

// TestTemplateGolden proves acceptance line 4 of P7-HOST-06: the include file the provider verifies
// against (DefaultRendering of the newest pinned commit) is byte-for-byte the golden that
// `providers gitlab init --stdout` is held to; it carries the GENERATED marker and the template version.
func TestTemplateGolden(t *testing.T) {
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	commits, err := gitlabtmpl.SourceCommits()
	if err != nil {
		t.Fatal(err)
	}
	direct, err := gitlabtmpl.Render(gitlabtmpl.Options{AgentCommit: commits[len(commits)-1]})
	if err != nil || string(direct) != string(golden) {
		t.Fatalf("render differs from the golden: %v", err)
	}
	viaProvider, err := pipeline.DefaultRendering(context.Background(), gitlabtmpl.Current, golden)
	if err != nil || string(viaProvider) != string(golden) {
		t.Fatalf("DefaultRendering differs from the golden: %v", err)
	}
	if !strings.HasPrefix(string(golden), gitlabtmpl.HeaderMarker+"\n") || gitlabtmpl.VersionOf(golden) != gitlabtmpl.Current {
		t.Errorf("marker or version missing")
	}
	again, _ := pipeline.DefaultRendering(context.Background(), gitlabtmpl.Current, golden)
	if string(again) != string(golden) {
		t.Error("rendering is not byte-stable")
	}
}

// TestDefaultRenderingPins proves a file naming a commit outside the allowlist is rendered with the
// newest allowed one (so it differs and is refused as edited), one naming none is refused, and an
// unknown template version is refused.
func TestDefaultRenderingPins(t *testing.T) {
	other, _ := gitlabtmpl.Render(gitlabtmpl.Options{AgentCommit: strings.Repeat("b", 40)})
	out, err := pipeline.DefaultRendering(context.Background(), 1, other)
	if err != nil || string(out) == string(other) {
		t.Errorf("an unlisted commit was rendered as given: %v", err)
	}
	if _, err = pipeline.DefaultRendering(context.Background(), 1, []byte("no commit here\n")); err == nil {
		t.Error("a file without a commit was rendered")
	}
	if _, err = pipeline.DefaultRendering(context.Background(), 9, other); err == nil {
		t.Error("template v9 was rendered")
	}
}
