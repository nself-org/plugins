package gates

// Purpose: the Swift and Xcode checks (P7-CI-64, Epic D33): swift-build and
// swift-test for SwiftPM packages, xcode-test per shared Xcode scheme.
// Inputs:  detector facts stack swiftpm and xcode_scheme (Name = scheme,
// Path = the .xcscheme file inside the project or workspace).
// Outputs: CheckDefs, and planned checks whose Target is the scheme name.
// Constraints: all three are required by default; xcode-test is bound to
// darwin/arm64 and requires.tools [xcode], so placement picks a Mac with
// Xcode and a host without it gets unplaceable.tool, never a pass; when a
// directory holds several Xcode containers each scheme names its own
// (-workspace over -project), otherwise xcodebuild finds the one container.

import (
	"path"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/detect"
)

type addFn = func(id, dir, target string, mods ...func(*PlannedCheck))

func swiftDefs() []CheckDef {
	fix := "Install the Swift toolchain (Xcode on macOS, swift.org on Linux)"
	xcode := def("xcode-test", "test", "xcodebuild", []string{"xcodebuild", "test", "-scheme", "{target}", "-destination", "platform=macOS"}, true, true, "deep", "Run on a Mac with Xcode installed (xcodebuild on PATH)")
	xcode.VersionProbe = []string{"xcodebuild", "-version"}
	xcode.Platform = "darwin/arm64"
	xcode.RequiresTools = []string{"xcode"}
	return []CheckDef{
		def("swift-build", "build", "swift", []string{"swift", "build"}, true, true, "deep", fix),
		def("swift-test", "test", "swift", []string{"swift", "test"}, true, true, "fast", fix),
		xcode,
	}
}

// xcodeSchemes answers, per scheme, which container xcodebuild must name.
type xcodeSchemes struct {
	containers  map[string]map[string]bool // workdir -> container dirs
	inWorkspace map[string]bool            // workdir + NUL + scheme: a workspace shares it
}

func xcodeIndex(facts []detect.Fact) xcodeSchemes {
	x := xcodeSchemes{containers: map[string]map[string]bool{}, inWorkspace: map[string]bool{}}
	for _, f := range facts {
		if f.Kind != "xcode_scheme" || f.Confidence != detect.Certain {
			continue
		}
		container := xcodeContainerOf(f.Path)
		work := path.Dir(container)
		if x.containers[work] == nil {
			x.containers[work] = map[string]bool{}
		}
		x.containers[work][container] = true
		if strings.HasSuffix(container, ".xcworkspace") {
			x.inWorkspace[work+"\x00"+f.Name] = true
		}
	}
	return x
}

// xcodeContainerOf maps App.xcodeproj/xcshareddata/xcschemes/S.xcscheme to App.xcodeproj.
func xcodeContainerOf(scheme string) string {
	return path.Dir(path.Dir(path.Dir(scheme)))
}

// plan adds xcode-test for one scheme fact.
func (x xcodeSchemes) plan(f detect.Fact, add addFn) {
	container := xcodeContainerOf(f.Path)
	work := path.Dir(container)
	if !strings.HasSuffix(container, ".xcworkspace") && x.inWorkspace[work+"\x00"+f.Name] {
		return // the workspace shares this scheme too: it plans the check
	}
	var argv []string
	if len(x.containers[work]) > 1 {
		if strings.HasSuffix(container, ".xcworkspace") {
			argv = []string{"xcodebuild", "test", "-workspace", path.Base(container), "-scheme", "{target}", "-destination", "platform=macOS"}
		} else {
			argv = []string{"xcodebuild", "test", "-project", path.Base(container), "-scheme", "{target}", "-destination", "platform=macOS"}
		}
	}
	add("xcode-test", work, f.Name, func(p *PlannedCheck) { p.Argv = argv })
}
