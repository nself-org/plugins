package detect

// Purpose: detector `swift` (P7-CI-64, Epic D33): SwiftPM packages and Xcode
// projects or workspaces with their shared schemes.
// Inputs:  the snapshot; Package.swift files, `*.xcodeproj/project.pbxproj`,
// `*.xcworkspace/contents.xcworkspacedata` and the `*.xcscheme` file names
// under `xcshareddata/xcschemes`. Project files are never parsed: a scheme is
// named by its file name, exactly as xcodebuild names it.
// Outputs: stack swiftpm (Path = Package.swift) with test_framework
// `swift test`; stack xcode (Path = the container's evidence file) with one
// xcode_scheme fact per shared scheme (Name = scheme, Path = the .xcscheme
// file). Package.resolved is digested by the lockfiles detector.
// Constraints: pure over the snapshot; an Xcode container without a shared
// scheme is an unknown manifest (the user must share a scheme), never a guess.

import (
	"path"
	"sort"
	"strings"
)

func detectSwift(s Snapshot) []Fact {
	var out []Fact
	for _, p := range s.Paths("Package.swift") {
		b, err := s.read(p)
		if err != nil {
			out = append(out, unknown("manifest", p, "unreadable Package.swift"))
			continue
		}
		if !validSwiftPackage(b) {
			out = append(out, unknown("manifest", p, "invalid Package.swift: no swift-tools-version line or Package(...) declaration"))
			continue
		}
		out = append(out, fact("stack", "swiftpm", p), fact("test_framework", "swift test", p))
	}
	return append(out, detectXcode(s)...)
}

func validSwiftPackage(b []byte) bool {
	text := string(b)
	return strings.Contains(text, "swift-tools-version") || strings.Contains(text, "Package(")
}

// xcodeContainer is an Xcode project or workspace and its evidence file.
type xcodeContainer struct{ dir, evidence string }

func detectXcode(s Snapshot) []Fact {
	var boxes []xcodeContainer
	for _, p := range s.Paths("project.pbxproj") {
		if dir := path.Dir(p); strings.HasSuffix(dir, ".xcodeproj") {
			boxes = append(boxes, xcodeContainer{dir, p})
		}
	}
	for _, p := range s.Paths("contents.xcworkspacedata") {
		dir := path.Dir(p)
		// The workspace embedded in every project (`X.xcodeproj/project.xcworkspace`) is not a container of its own.
		if strings.HasSuffix(dir, ".xcworkspace") && !strings.Contains(dir, ".xcodeproj/") {
			boxes = append(boxes, xcodeContainer{dir, p})
		}
	}
	sort.Slice(boxes, func(i, j int) bool { return boxes[i].dir < boxes[j].dir })
	var out []Fact
	for _, c := range boxes {
		out = append(out, fact("stack", "xcode", c.evidence))
		schemes := s.sharedSchemes(c.dir)
		if len(schemes) == 0 {
			out = append(out, unknown("manifest", c.evidence, "Xcode container has no shared scheme: share one in Xcode (Manage Schemes) to gate it"))
			continue
		}
		for _, p := range schemes {
			out = append(out, fact("xcode_scheme", strings.TrimSuffix(path.Base(p), ".xcscheme"), p))
		}
	}
	return out
}

// sharedSchemes lists the .xcscheme files shared by an Xcode container.
func (s Snapshot) sharedSchemes(container string) []string {
	prefix := container + "/xcshareddata/xcschemes/"
	var out []string
	for p := range s.files {
		if strings.HasPrefix(p, prefix) && strings.HasSuffix(p, ".xcscheme") && !strings.Contains(p[len(prefix):], "/") {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}
