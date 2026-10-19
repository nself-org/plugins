package plan

// Purpose: tell the user why an Xcode container yields no xcode-test check
// (D-1028). Xcode leaves schemes unshared by default and the detector never
// guesses a scheme, so such a container plans nothing; without a note the
// gate looks green by omission.
// Inputs:  the detector facts in Input and the config's explicit jobs.
// Outputs: one Skipped entry per container, job `xcode-test:<container>`,
// reason advisory.xcode_no_shared_scheme, detail naming the remedy.
// Constraints: pure; a config that declares its own jobs gets no note (the
// catalogue is not in use); a project that a workspace with a shared scheme
// lists is built through that workspace, so it needs no note.

import (
	"path"
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/detect"
)

func xcodeNotes(in Input) []Skipped {
	out := []Skipped{}
	if len(in.Config.Jobs) > 0 {
		return out
	}
	schemed := map[string]bool{}  // containers holding a shared scheme
	member := map[string]string{} // project container -> the workspace listing it
	for _, f := range in.Facts {
		switch {
		case f.Kind == "xcode_scheme" && f.Confidence == detect.Certain:
			// App.xcodeproj/xcshareddata/xcschemes/S.xcscheme -> App.xcodeproj
			schemed[path.Dir(path.Dir(path.Dir(f.Path)))] = true
		case f.Kind == "xcode_member" && f.Confidence == detect.Certain:
			member[f.Name] = path.Dir(f.Path)
		}
	}
	seen := map[string]bool{}
	for _, f := range in.Facts {
		if f.Kind != "manifest" || f.Confidence != detect.Unknown || f.Reason != detect.XcodeNoSharedScheme {
			continue
		}
		container := path.Dir(f.Path)
		if seen[container] || schemed[member[container]] {
			continue
		}
		seen[container] = true
		out = append(out, Skipped{
			Job:    "xcode-test:" + container,
			Reason: "advisory.xcode_no_shared_scheme",
			Detail: container + " has no shared scheme, so no xcode-test is planned: share a scheme in Xcode (Product > Scheme > Manage Schemes > Shared) or declare the check under `jobs:`",
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Job < out[j].Job })
	return out
}
