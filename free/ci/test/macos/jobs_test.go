//go:build darwin && macosleg

package macos

// Purpose: jobs on the leg run as the job user (Epic D20 acceptance 2 and 7): through the same sudo chain
// the agent uses (nself-ci -> sudo -> job-shim), a shell job sees `nself-ci-job-1` and a HOME inside its
// job root, the SwiftPM fixture and the Xcode fixture (a framework-shaped package with an XCTest bundle
// and no host app, built by xcodebuild through its package scheme) pass as that user, a planted failing
// XCTest fails the run, and the job leaves nothing outside its job root except what the root holds.
// Constraints: the fixture lives in testdata/fixture (its manifest is stored as Package.swift.fixture so
// the repository's own swift detector does not pick it up); everything a job writes goes under its job
// root because HOME, TMPDIR and the module caches point there.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// shimJob runs argv in dir as job user n through the shim; env is added to a minimal base whose HOME,
// TMPDIR and cache variables point into the job root.
func (l *leg) shimJob(n int, dir string, argv ...string) (string, int) {
	l.t.Helper()
	root := jobRoot(n)
	env := []string{"HOME=" + root + "/home", "TMPDIR=" + root + "/tmp", "PATH=/usr/bin:/bin:/usr/sbin:/sbin",
		"CLANG_MODULE_CACHE_PATH=" + root + "/home/.cache/clang", "XDG_CACHE_HOME=" + root + "/home/.cache"}
	req, err := json.Marshal(map[string]any{"argv": argv, "dir": dir, "env": env})
	if err != nil {
		l.t.Fatal(err)
	}
	file := filepath.Join(l.home, "job-request.json")
	if err = os.WriteFile(file, append(req, '\n'), 0o644); err != nil {
		l.t.Fatal(err)
	}
	return l.sudo("cat " + file + " | " + asAgent(n) + " 2>&1")
}

// stageFixture copies the fixture into the job root of slot n as pkg (variant "" or "failing").
func (l *leg) stageFixture(n int, name, variant string) string {
	l.t.Helper()
	src, err := filepath.Abs("testdata/fixture")
	if err != nil {
		l.t.Fatal(err)
	}
	root := jobRoot(n)
	pkg := root + "/" + name
	l.mustSudo("mkdir -p " + root + "/home " + root + "/tmp " + pkg + " && cp -R " + src + "/. " + pkg + "/ && mv " + pkg + "/Package.swift.fixture " + pkg + "/Package.swift")
	if variant == "failing" {
		l.mustSudo("sed -i '' 's/XCTAssertEqual(add(2, 3), 5)/XCTAssertEqual(add(2, 3), 6)/' " + pkg + "/Tests/FixtureTests/FixtureTests.swift")
	}
	l.mustSudo("chown -R " + jobUser(n) + " " + root + " && chmod 700 " + root)
	return pkg
}

func (l *leg) jobs() {
	t := l.t
	root := jobRoot(1)
	pkg := l.stageFixture(1, "pkg", "")
	out, rc := l.shimJob(1, pkg, "/bin/sh", "-c", "id -un; echo $HOME; pwd")
	if want := jobUser(1) + "\n" + root + "/home\n" + pkg; rc != 0 || out != want {
		t.Fatalf("shell job: rc=%d\n%q\nwant %q", rc, out, want)
	}
	if out, rc = l.shimJob(1, pkg, "/usr/bin/swift", "test"); rc != 0 || !strings.Contains(out, "Executed 1 test") {
		t.Fatalf("SwiftPM fixture as %s: rc=%d\n%s", jobUser(1), rc, out)
	}
	if out, rc = l.shimJob(1, pkg, "/usr/bin/xcodebuild", "test", "-scheme", "Fixture", "-destination", "platform=macOS"); rc != 0 || !strings.Contains(out, "TEST SUCCEEDED") {
		t.Fatalf("Xcode fixture as %s: rc=%d\n%s", jobUser(1), rc, out)
	}
	if dd, _ := l.sudo("ls " + root + "/home/Library/Developer/Xcode/DerivedData"); dd == "" {
		t.Error("xcodebuild wrote no DerivedData under the job HOME")
	}
	bad := l.stageFixture(1, "failing", "failing")
	if out, rc = l.shimJob(1, bad, "/usr/bin/swift", "test"); rc == 0 || !strings.Contains(out, "XCTAssertEqual failed") {
		t.Errorf("a failing XCTest did not fail the run: rc=%d\n%s", rc, out)
	}
	// Nothing the job wrote may live outside its job root: caches, DerivedData and temp files go to HOME/TMPDIR.
	stray, _ := l.sudo("find /private/var/folders /private/var/tmp /private/tmp /Users /Library -xdev -user " + jobUser(1) + " 2>/dev/null | head -n 5")
	if stray != "" {
		t.Errorf("the job left files outside its job root:\n%s", stray)
	}
	// The attempt ends: its job root is emptied, nothing of it remains under HOME.
	l.mustSudo("find " + root + " -mindepth 1 -delete")
	if left, _ := l.sudo("find " + root + " -mindepth 1 | head -n 3"); left != "" {
		t.Errorf("files survived the cleanup: %s", left)
	}
}
