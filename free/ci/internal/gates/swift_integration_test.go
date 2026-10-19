//go:build integration

package gates

// Purpose: the SwiftPM fixture on any host with Swift (P7-CI-64): swift-build
// and swift-test pass on the package, a failing XCTest fails swift-test, and
// a host without swift gets E614 instead of a pass. Linux evidence comes from
// the `detect-langs (ubuntu-latest)` job, which runs it with INTEGRATION=1.
// Constraints: build tag integration; a missing prerequisite is t.Fatal.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestSwiftPMFixtures(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Fatal("TestSwiftPMFixtures needs INTEGRATION=1")
	}
	if _, err := exec.LookPath("swift"); err != nil {
		t.Fatalf("swift must be on PATH of the verify host: %v", err)
	}
	realPath := os.Getenv("PATH")

	t.Run("package builds and its tests pass", func(t *testing.T) {
		root := copyFixtureTree(t, "swift", "swiftpm")
		checks := planned(t, root)
		build, test := runCheck(t, root, checks["swift-build"]), runCheck(t, root, checks["swift-test"])
		if build.Result != "pass" || test.Result != "pass" || !build.Required || !test.Required {
			t.Fatalf("swift-build %+v swift-test %+v", build, test)
		}
	})

	t.Run("a failing XCTest fails swift-test", func(t *testing.T) {
		root := copyFixtureTree(t, "swift", "swiftpm")
		bad := "import XCTest\n@testable import Calc\n\nfinal class CalcTests: XCTestCase {\n    func testTotal() {\n        XCTAssertEqual(total(1, 2), 4)\n    }\n}\n"
		if err := os.WriteFile(filepath.Join(root, "Tests", "CalcTests", "CalcTests.swift"), []byte(bad), 0o644); err != nil {
			t.Fatal(err)
		}
		c := runCheck(t, root, planned(t, root)["swift-test"])
		if c.Result != "fail" || c.FailureClass != "code" || model.Verdict([]model.Check{c}) != "fail" {
			t.Fatalf("failing XCTest: %+v", c)
		}
	})

	t.Run("no swift on the host is E614", func(t *testing.T) {
		root := copyFixtureTree(t, "swift", "swiftpm")
		p := planned(t, root)["swift-test"]
		t.Setenv("PATH", t.TempDir())
		c := runCheck(t, root, p)
		t.Setenv("PATH", realPath)
		if c.Result != "error" || !strings.Contains(c.Excerpt, "E614") {
			t.Fatalf("swift-test without swift: %+v", c)
		}
	})
}
