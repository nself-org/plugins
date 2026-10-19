package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

// checkGolden detects the fixture testdata/<group>/<name> and compares the
// fact JSON with testdata/<group>/<name>.golden.json (UPDATE_GOLDEN=1 rewrites it).
func checkGolden(t *testing.T, group, name string) []Fact {
	t.Helper()
	facts, err := DefaultRegistry().Detect(os.DirFS(filepath.Join("testdata", group, name)), allChecks)
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.MarshalIndent(facts, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	golden := filepath.Join("testdata", group, name+".golden.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("golden mismatch for %s/%s:\n%s", group, name, got)
	}
	return facts
}

func has(facts []Fact, kind, name, path string) bool {
	for _, f := range facts {
		if f.Kind == kind && f.Name == name && f.Path == path && f.Confidence == Certain {
			return true
		}
	}
	return false
}

func TestSwiftFixtures(t *testing.T) {
	for _, name := range []string{"swiftpm", "xcode-app", "monorepo"} {
		t.Run(name, func(t *testing.T) {
			facts := checkGolden(t, "swift", name)
			switch name {
			case "swiftpm":
				if !has(facts, "stack", "swiftpm", "Package.swift") || !has(facts, "lockfile", "swiftpm", "Package.resolved") {
					t.Fatalf("SwiftPM package facts missing: %+v", facts)
				}
			case "xcode-app":
				if !has(facts, "xcode_scheme", "App", "App.xcodeproj/xcshareddata/xcschemes/App.xcscheme") {
					t.Fatalf("shared scheme missing: %+v", facts)
				}
			case "monorepo":
				if !has(facts, "stack", "swiftpm", "Package.swift") || !has(facts, "xcode_scheme", "App", "app/App.xcodeproj/xcshareddata/xcschemes/App.xcscheme") {
					t.Fatalf("monorepo facts missing: %+v", facts)
				}
			}
		})
	}
}

func file(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }

func TestSwiftEdgeCases(t *testing.T) {
	scheme := "App.xcodeproj/xcshareddata/xcschemes/App.xcscheme"
	cases := []struct {
		name  string
		files fstest.MapFS
		check func(t *testing.T, facts []Fact)
	}{
		{"invalid Package.swift is unknown", fstest.MapFS{"Package.swift": file("not a package")}, func(t *testing.T, facts []Fact) {
			for _, f := range facts {
				if f.Kind == "stack" {
					t.Fatalf("invalid manifest produced a stack: %+v", f)
				}
			}
			if !strings.Contains(factsJSON(facts), `"unknown"`) {
				t.Fatalf("no unknown manifest fact: %+v", facts)
			}
		}},
		{"project without shared scheme is unknown", fstest.MapFS{"App.xcodeproj/project.pbxproj": file("x")}, func(t *testing.T, facts []Fact) {
			if !has(facts, "stack", "xcode", "App.xcodeproj/project.pbxproj") {
				t.Fatalf("xcode stack missing: %+v", facts)
			}
			for _, f := range facts {
				if f.Kind == "xcode_scheme" {
					t.Fatalf("scheme invented: %+v", f)
				}
				if f.Kind == "manifest" && f.Confidence != Unknown {
					t.Fatalf("scheme gap must be unknown: %+v", f)
				}
			}
		}},
		{"embedded workspace is not a container", fstest.MapFS{
			"App.xcodeproj/project.pbxproj":                              file("x"),
			"App.xcodeproj/project.xcworkspace/contents.xcworkspacedata": file("x"),
			scheme: file("x"),
		}, func(t *testing.T, facts []Fact) {
			n := 0
			for _, f := range facts {
				if f.Kind == "stack" && f.Name == "xcode" {
					n++
				}
			}
			if n != 1 {
				t.Fatalf("want one xcode stack, got %d: %+v", n, facts)
			}
		}},
		{"build output is ignored", fstest.MapFS{
			"Package.swift":                               file("// swift-tools-version:5.9\n"),
			".build/checkouts/dep/Package.swift":          file("// swift-tools-version:5.9\n"),
			"Pods/Dep/Package.swift":                      file("// swift-tools-version:5.9\n"),
			"DerivedData/X/App.xcodeproj/project.pbxproj": file("x"),
		}, func(t *testing.T, facts []Fact) {
			for _, f := range facts {
				if f.Kind == "stack" && f.Path != "Package.swift" {
					t.Fatalf("ignored directory was scanned: %+v", f)
				}
			}
		}},
		{"package without tests has no test framework", fstest.MapFS{
			"Package.swift":     file("// swift-tools-version:5.9\nlet package = Package(name: \"X\")\n"),
			"Sources/X/X.swift": file("public let x = 1\n"),
		}, func(t *testing.T, facts []Fact) {
			if !has(facts, "stack", "swiftpm", "Package.swift") || has(facts, "test_framework", "swift test", "Package.swift") {
				t.Fatalf("swiftpm without tests: %+v", facts)
			}
		}},
		{"package with a Tests directory has tests", fstest.MapFS{
			"Package.swift":             file("// swift-tools-version:5.9\n"),
			"Tests/XTests/XTests.swift": file("import XCTest\n"),
		}, func(t *testing.T, facts []Fact) {
			if !has(facts, "test_framework", "swift test", "Package.swift") {
				t.Fatalf("Tests/ directory not seen: %+v", facts)
			}
		}},
		{"SwiftPM's own workspace is not an Xcode container", fstest.MapFS{
			"Package.swift": file("// swift-tools-version:5.9\n"),
			".swiftpm/xcode/package.xcworkspace/contents.xcworkspacedata": file("x"),
		}, func(t *testing.T, facts []Fact) {
			for _, f := range facts {
				if f.Name == "xcode" || f.Kind == "manifest" {
					t.Fatalf("SwiftPM workspace taken for an Xcode container: %+v", f)
				}
			}
		}},
		{"scheme file names a scheme, nested files do not", fstest.MapFS{
			"App.xcodeproj/project.pbxproj":                           file("x"),
			"App.xcodeproj/xcshareddata/xcschemes/Two Words.xcscheme": file("x"),
			"App.xcodeproj/xcshareddata/xcschemes/old/Old.xcscheme":   file("x"),
		}, func(t *testing.T, facts []Fact) {
			if !has(facts, "xcode_scheme", "Two Words", "App.xcodeproj/xcshareddata/xcschemes/Two Words.xcscheme") {
				t.Fatalf("scheme with a space missing: %+v", facts)
			}
			for _, f := range facts {
				if f.Name == "Old" {
					t.Fatalf("nested scheme file counted: %+v", f)
				}
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			facts, err := DefaultRegistry().Detect(c.files, allChecks)
			if err != nil {
				t.Fatal(err)
			}
			c.check(t, facts)
		})
	}
}

func factsJSON(facts []Fact) string {
	b, _ := json.Marshal(facts)
	return string(b)
}

// TestSwiftDetectorBoundToChecks: the detectors run only when a bound check exists.
func TestSwiftDetectorBoundToChecks(t *testing.T) {
	files := fstest.MapFS{"Package.swift": file("// swift-tools-version:5.9\n"), "pyproject.toml": file("[project]\nname='x'\n")}
	facts, err := DefaultRegistry().Detect(files, func(id string) bool { return id == "go.test" })
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range facts {
		if f.Name == "swiftpm" || f.Name == "python" {
			t.Fatalf("unbound detector ran: %+v", f)
		}
	}
}
