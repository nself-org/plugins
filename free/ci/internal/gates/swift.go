package gates

// Purpose: the Swift and Xcode checks (P7-CI-64, Epic D33): swift-build and
// swift-test for SwiftPM packages, xcode-test per shared Xcode scheme.
// Inputs:  detector facts stack swiftpm and xcode_scheme (Name = scheme,
// Path = the .xcscheme file inside the project or workspace).
// Outputs: CheckDefs, and planned checks whose Target is the scheme name.
// Constraints: all three are required by default; xcode-test is bound to
// darwin/arm64 and requires.tools [xcode], so placement picks a Mac with
// Xcode and a host without it gets unplaceable.tool, never a pass; when a
// directory holds several Xcode containers each check names its own
// (-workspace or -project), otherwise xcodebuild finds the one container.

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

// swiftTestIndex lists the Package.swift paths whose package has tests to run.
func swiftTestIndex(facts []detect.Fact) map[string]bool {
	out := map[string]bool{}
	for _, f := range facts {
		if f.Kind == "test_framework" && f.Name == "swift test" && f.Confidence == detect.Certain {
			out[f.Path] = true
		}
	}
	return out
}

// xcodeSchemes answers, per scheme, which container xcodebuild must name.
type xcodeSchemes struct {
	containers map[string]map[string]bool // workdir -> container dirs
	member     map[string]string          // project container -> the workspace that lists it
}

func xcodeIndex(facts []detect.Fact) xcodeSchemes {
	x := xcodeSchemes{containers: map[string]map[string]bool{}, member: map[string]string{}}
	known := func(container string) {
		work := path.Dir(container)
		if x.containers[work] == nil {
			x.containers[work] = map[string]bool{}
		}
		x.containers[work][container] = true
	}
	for _, f := range facts {
		if f.Confidence != detect.Certain {
			continue
		}
		switch {
		case f.Kind == "stack" && f.Name == "xcode":
			known(path.Dir(f.Path)) // every container counts, shared schemes or not
		case f.Kind == "xcode_scheme":
			known(xcodeContainerOf(f.Path))
		case f.Kind == "xcode_member":
			x.member[f.Name] = path.Dir(f.Path)
		}
	}
	return x
}

// xcodeContainerOf maps App.xcodeproj/xcshareddata/xcschemes/S.xcscheme to App.xcodeproj.
func xcodeContainerOf(scheme string) string {
	return path.Dir(path.Dir(path.Dir(scheme)))
}

// plan adds xcode-test for one scheme fact. A directory with several
// containers names each one explicitly, and the container joins the target so
// two projects sharing a scheme name both stay in the plan.
func (x xcodeSchemes) plan(f detect.Fact, add addFn) {
	container := xcodeContainerOf(f.Path)
	if ws, ok := x.member[container]; ok {
		container = ws // a project's scheme builds through the workspace that lists it
	}
	work := path.Dir(container)
	target, argv := f.Name, []string(nil)
	if len(x.containers[work]) > 1 {
		flag := "-project"
		if strings.HasSuffix(container, ".xcworkspace") {
			flag = "-workspace"
		}
		target = path.Base(container) + "/" + f.Name
		argv = []string{"xcodebuild", "test", flag, path.Base(container), "-scheme", f.Name, "-destination", "platform=macOS"}
	}
	add("xcode-test", work, target, func(p *PlannedCheck) { p.Argv = argv })
}
