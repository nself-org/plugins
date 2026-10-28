//go:build darwin_integration

package gates

// Purpose: the Xcode and SwiftPM fixtures on a Mac with Xcode (P7-CI-64):
// xcode-test passes on the app fixture, a failing XCTest fails it, swift-build
// and swift-test pass on the package, and a Mac without xcodebuild gets E614.
// The `detect-langs (macos-15)` job runs it on GitHub's macOS image.
// Constraints: build tag darwin_integration; a missing prerequisite is t.Fatal.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/detect"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// darwinPlanned detects root and returns its planned checks by id (this file
// builds under its own tag, so it does not share the integration helpers).
func darwinPlanned(t *testing.T, root string) map[string]PlannedCheck {
	t.Helper()
	r := DefaultRegistry()
	facts, err := detect.DefaultRegistry().Detect(os.DirFS(root), r.Has)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]PlannedCheck{}
	for _, p := range r.Plan(facts, Options{}) {
		out[p.Def.ID] = p
	}
	return out
}

func darwinRun(t *testing.T, root string, p PlannedCheck) model.Check {
	t.Helper()
	return (Runner{Executor: &ciexec.Executor{Isolation: ciexec.BaselineIsolator{}}, Home: t.TempDir()}).Run(context.Background(), root, p, model.PipelineConfig{})
}

func TestSwiftDarwinFixtures(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Fatalf("TestSwiftDarwinFixtures runs on macOS, not %s", runtime.GOOS)
	}
	for _, tool := range []string{"xcodebuild", "swift"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("%s must be on PATH of the verify host: %v", tool, err)
		}
	}
	realPath := os.Getenv("PATH")

	t.Run("xcode-test passes on the app fixture", func(t *testing.T) {
		root := copyFixtureTree(t, "swift", "xcode-app")
		c := darwinRun(t, root, darwinPlanned(t, root)["xcode-test"])
		if c.Result != "pass" || !c.Required {
			t.Fatalf("xcode-test: %+v", c)
		}
	})

	t.Run("a failing XCTest fails xcode-test", func(t *testing.T) {
		root := copyFixtureTree(t, "swift", "xcode-app")
		bad := "import XCTest\n@testable import App\n\nfinal class AppTests: XCTestCase {\n    func testTotal() {\n        XCTAssertEqual(total(1, 2), 4)\n    }\n}\n"
		if err := os.WriteFile(filepath.Join(root, "AppTests", "AppTests.swift"), []byte(bad), 0o644); err != nil {
			t.Fatal(err)
		}
		c := darwinRun(t, root, darwinPlanned(t, root)["xcode-test"])
		if c.Result != "fail" || c.FailureClass != "code" || model.Verdict([]model.Check{c}) != "fail" {
			t.Fatalf("failing XCTest: %+v", c)
		}
	})

	t.Run("SwiftPM fixture passes on macOS", func(t *testing.T) {
		root := copyFixtureTree(t, "swift", "swiftpm")
		checks := darwinPlanned(t, root)
		if b, tt := darwinRun(t, root, checks["swift-build"]), darwinRun(t, root, checks["swift-test"]); b.Result != "pass" || tt.Result != "pass" {
			t.Fatalf("swift-build %+v swift-test %+v", b, tt)
		}
	})

	t.Run("no xcodebuild on the Mac is E614", func(t *testing.T) {
		root := copyFixtureTree(t, "swift", "xcode-app")
		p := darwinPlanned(t, root)["xcode-test"]
		t.Setenv("PATH", t.TempDir())
		c := darwinRun(t, root, p)
		t.Setenv("PATH", realPath)
		if c.Result != "error" || !strings.Contains(c.Excerpt, "E614") || !strings.Contains(c.Excerpt, "Xcode") {
			t.Fatalf("xcode-test without xcodebuild: %+v", c)
		}
	})
}
