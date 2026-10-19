package plan

// D-1028: an Xcode container with no shared scheme plans no xcode-test; the
// plan must say so (and name the remedy) instead of staying silent.

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/detect"
)

func xcodeFacts(extra ...detect.Fact) []detect.Fact {
	return append([]detect.Fact{
		{Kind: "stack", Name: "xcode", Path: "App.xcodeproj/project.pbxproj", Confidence: detect.Certain},
		{Kind: "manifest", Name: "unknown", Path: "App.xcodeproj/project.pbxproj", Confidence: detect.Unknown, Reason: detect.XcodeNoSharedScheme},
	}, extra...)
}

func TestXcodeWithoutSharedSchemeIsNoted(t *testing.T) {
	p := mustBuild(t, Input{Facts: xcodeFacts(), World: linuxWorld()})
	var got *Skipped
	for i := range p.Skipped {
		if p.Skipped[i].Job == "xcode-test:App.xcodeproj" {
			got = &p.Skipped[i]
		}
	}
	if got == nil {
		t.Fatalf("no skipped note for the unshared Xcode container: %+v", p.Skipped)
	}
	if got.Reason != "advisory.xcode_no_shared_scheme" || !strings.Contains(got.Detail, "share") || !strings.Contains(got.Detail, "jobs:") {
		t.Errorf("note = %+v, want the reason and a remedy naming scheme sharing and the jobs: override", *got)
	}
	if !strings.Contains(strings.Join(p.Human(nil), "\n"), "App.xcodeproj") {
		t.Error("the human plan does not show the note")
	}
}

func TestXcodeWithSharedSchemeHasNoNote(t *testing.T) {
	scheme := detect.Fact{Kind: "xcode_scheme", Name: "App", Path: "App.xcodeproj/xcshareddata/xcschemes/App.xcscheme", Confidence: detect.Certain}
	facts := []detect.Fact{xcodeFacts()[0], scheme}
	p := mustBuild(t, Input{Facts: facts, World: linuxWorld()})
	for _, s := range p.Skipped {
		if strings.HasPrefix(s.Job, "xcode-test") {
			t.Errorf("unexpected note %+v", s)
		}
	}
}

func TestXcodeProjectInSchemedWorkspaceHasNoNote(t *testing.T) {
	facts := xcodeFacts(
		detect.Fact{Kind: "stack", Name: "xcode", Path: "App.xcworkspace/contents.xcworkspacedata", Confidence: detect.Certain},
		detect.Fact{Kind: "xcode_member", Name: "App.xcodeproj", Path: "App.xcworkspace/contents.xcworkspacedata", Confidence: detect.Certain},
		detect.Fact{Kind: "xcode_scheme", Name: "App", Path: "App.xcworkspace/xcshareddata/xcschemes/App.xcscheme", Confidence: detect.Certain},
	)
	p := mustBuild(t, Input{Facts: facts, World: linuxWorld()})
	for _, s := range p.Skipped {
		if strings.HasPrefix(s.Job, "xcode-test") {
			t.Errorf("unexpected note %+v", s)
		}
	}
}
