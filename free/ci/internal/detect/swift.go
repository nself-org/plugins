package detect

// Purpose: detector `swift` (P7-CI-64, Epic D33): SwiftPM packages and Xcode
// projects or workspaces with their shared schemes.
// Inputs:  the snapshot; Package.swift files, `*.xcodeproj/project.pbxproj`,
// `*.xcworkspace/contents.xcworkspacedata` and the `*.xcscheme` file names
// under `xcshareddata/xcschemes`. Project files are never parsed: a scheme is
// named by its file name, exactly as xcodebuild names it.
// Outputs: stack swiftpm (Path = Package.swift) with test_framework
// `swift test` when the package has a test target or a Tests/ directory; stack xcode (Path = the container's evidence file) with one
// xcode_scheme fact per shared scheme (Name = scheme, Path = the .xcscheme
// file). Package.resolved is digested by the lockfiles detector.
// Constraints: pure over the snapshot; the workspace SwiftPM keeps under
// `.swiftpm/` is not an Xcode container; an Xcode container without a shared
// scheme is an unknown manifest (the user must share a scheme), never a guess.

import (
	"encoding/xml"
	"path"
	"regexp"
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
		out = append(out, fact("stack", "swiftpm", p))
		if hasSwiftTests(s, p, string(b)) {
			out = append(out, fact("test_framework", "swift test", p))
		}
	}
	return append(out, detectXcode(s)...)
}

var testTarget = regexp.MustCompile(`\.testTarget\s*\(`)

// hasSwiftTests reports whether the package declares a test target or has a
// Tests/ directory: `swift test` fails on a package with nothing to run.
func hasSwiftTests(s Snapshot, manifest, text string) bool {
	if testTarget.MatchString(text) {
		return true
	}
	prefix := path.Join(path.Dir(manifest), "Tests") + "/"
	for p := range s.files {
		if strings.HasPrefix(p, prefix) && strings.HasSuffix(p, ".swift") {
			return true
		}
	}
	return false
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
		if strings.HasSuffix(dir, ".xcworkspace") && !strings.Contains(dir, ".xcodeproj/") && !strings.Contains("/"+dir, "/.swiftpm/") {
			boxes = append(boxes, xcodeContainer{dir, p})
		}
	}
	sort.Slice(boxes, func(i, j int) bool { return boxes[i].dir < boxes[j].dir })
	var out []Fact
	for _, c := range boxes {
		out = append(out, fact("stack", "xcode", c.evidence))
		if strings.HasSuffix(c.dir, ".xcworkspace") {
			out = append(out, workspaceMembers(s, c, boxes)...)
		}
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

// wsNode is one element of contents.xcworkspacedata: FileRef and Group both
// carry a location attribute, Group nests more elements.
type wsNode struct {
	XMLName  xml.Name
	Location string   `xml:"location,attr"`
	Children []wsNode `xml:",any"`
}

// workspaceMembers names the detected projects a workspace lists
// (CocoaPods and SwiftPM monorepos build their app through the workspace):
// one xcode_member fact per project, Name = the project container, Path = the
// workspace file. group: references resolve against the enclosing group,
// container: references against the folder holding the workspace.
func workspaceMembers(s Snapshot, ws xcodeContainer, boxes []xcodeContainer) []Fact {
	b, err := s.read(ws.evidence)
	if err != nil {
		return nil
	}
	var root wsNode
	if xml.Unmarshal(b, &root) != nil {
		return nil
	}
	known := map[string]bool{}
	for _, c := range boxes {
		known[c.dir] = true
	}
	base := path.Dir(ws.dir)
	var out []Fact
	var walk func(n wsNode, group string)
	walk = func(n wsNode, group string) {
		for _, c := range n.Children {
			kind, ref, ok := strings.Cut(c.Location, ":")
			if !ok || (kind != "group" && kind != "container") {
				continue
			}
			at := path.Join(group, ref)
			if kind == "container" {
				at = path.Join(base, ref)
			}
			switch c.XMLName.Local {
			case "FileRef":
				if known[at] {
					out = append(out, fact("xcode_member", at, ws.evidence))
				}
			case "Group":
				walk(c, at)
			}
		}
	}
	walk(root, base)
	return out
}
