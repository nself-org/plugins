package main

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fakeRunner returns a canned exit code per script and records every call. It runs nothing.
type fakeRunner struct {
	codes map[string]int
	err   error
	dirs  []string
	calls []string
}

// Run records the call and returns the canned code for script, or 0 when none is set.
func (f *fakeRunner) Run(dir, script string) (int, error) {
	f.dirs = append(f.dirs, dir)
	f.calls = append(f.calls, script)
	if f.err != nil {
		return -1, f.err
	}
	return f.codes[script], nil
}

// fakeFixture hands out a fixed directory and counts creations and cleanups.
type fakeFixture struct {
	made    int
	cleaned int
	err     error
}

// fixture returns the fake Fixture function bound to f.
func (f *fakeFixture) fixture() (string, func(), error) {
	if f.err != nil {
		return "", nil, f.err
	}
	f.made++
	return "fixture-dir", func() { f.cleaned++ }, nil
}

// checkWith runs CheckPage on a testdata page with the fake runner and fixture.
func checkWith(t *testing.T, page string, r *fakeRunner, fx *fakeFixture) []Failure {
	t.Helper()
	failures, err := CheckPage("testdata/"+page, Options{Runner: r, Fixture: fx.fixture})
	if err != nil {
		t.Fatalf("CheckPage(%s): %v", page, err)
	}
	return failures
}

func TestCheckPagePasses(t *testing.T) {
	r := &fakeRunner{}
	fx := &fakeFixture{}
	if got := checkWith(t, "pass.md", r, fx); len(got) != 0 {
		t.Fatalf("failures = %v, want none", got)
	}
	if want := []string{"echo ok", "true"}; !reflect.DeepEqual(r.calls, want) {
		t.Errorf("ran %q, want %q (the sh block must be skipped)", r.calls, want)
	}
	if fx.made != 2 || fx.cleaned != 2 {
		t.Errorf("fixtures made=%d cleaned=%d, want 2 and 2 (one fresh fixture per block)", fx.made, fx.cleaned)
	}
	for _, d := range r.dirs {
		if d != "fixture-dir" {
			t.Errorf("block ran in %q, want the fixture dir", d)
		}
	}
}

func TestCheckPageReportsNonZeroExit(t *testing.T) {
	r := &fakeRunner{codes: map[string]int{"exit 3": 3, "exit 7": 7}}
	got := checkWith(t, "fail.md", r, &fakeFixture{})
	if len(got) != 2 {
		t.Fatalf("got %d failures, want 2: %v", len(got), got)
	}
	want := []Failure{
		{Page: "testdata/fail.md", Block: 2, ExitCode: 3, Message: "exit 3"},
		{Page: "testdata/fail.md", Block: 3, ExitCode: 7, Message: "exit 7"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("failures = %+v, want %+v", got, want)
	}
	if msg := got[0].Error(); msg != "testdata/fail.md: block 2: exit 3" {
		t.Errorf("Error() = %q", msg)
	}
	if msg := got[1].Error(); msg != "testdata/fail.md: block 3: exit 7" {
		t.Errorf("Error() = %q", msg)
	}
}

func TestCheckPageExpectFailMarker(t *testing.T) {
	r := &fakeRunner{codes: map[string]int{"exit 1": 1, "exit 5": 5}}
	got := checkWith(t, "expect-fail.md", r, &fakeFixture{})
	if len(got) != 2 {
		t.Fatalf("got %d failures, want 2: %v", len(got), got)
	}
	if got[0].Block != 2 || got[0].ExitCode != 0 || got[0].Message != "exit 0 (expected non-zero: marked expect-fail)" {
		t.Errorf("marked zero-exit failure = %+v", got[0])
	}
	if msg := got[0].Error(); msg != "testdata/expect-fail.md: block 2: exit 0 (expected non-zero: marked expect-fail)" {
		t.Errorf("Error() = %q", msg)
	}
	if got[1].Block != 3 || got[1].ExitCode != 5 || got[1].Message != "exit 5" {
		t.Errorf("unmarked block failure = %+v, want block 3 exit 5", got[1])
	}
}

func TestCheckPageWithoutBashBlocksRunsNothing(t *testing.T) {
	r := &fakeRunner{}
	fx := &fakeFixture{}
	if got := checkWith(t, "no-blocks.md", r, fx); len(got) != 0 {
		t.Fatalf("failures = %v, want none", got)
	}
	if len(r.calls) != 0 || fx.made != 0 {
		t.Errorf("runner calls=%d fixtures=%d, want 0 and 0", len(r.calls), fx.made)
	}
}

func TestCheckPageRunnerError(t *testing.T) {
	r := &fakeRunner{err: errors.New("boom")}
	got := checkWith(t, "pass.md", r, &fakeFixture{})
	if len(got) != 2 {
		t.Fatalf("got %d failures, want 2: %v", len(got), got)
	}
	if got[0].ExitCode != -1 || got[0].Message != "could not run: boom" {
		t.Errorf("runner error failure = %+v", got[0])
	}
}

func TestCheckPageFixtureError(t *testing.T) {
	r := &fakeRunner{}
	got := checkWith(t, "pass.md", r, &fakeFixture{err: errors.New("no disk")})
	if len(got) != 2 || got[0].Message != "fixture: no disk" {
		t.Fatalf("fixture failures = %v", got)
	}
	if len(r.calls) != 0 {
		t.Errorf("runner ran %d blocks without a fixture", len(r.calls))
	}
}

func TestCheckPageMissingFile(t *testing.T) {
	_, err := CheckPage("testdata/missing.md", Options{Runner: &fakeRunner{}, Fixture: (&fakeFixture{}).fixture})
	if err == nil {
		t.Fatal("CheckPage on a missing page returned no error")
	}
}

func TestRunUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(nil, &out, &errOut); code != 2 {
		t.Errorf("run() = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "usage: check-doc-commands.sh <page.md>...") {
		t.Errorf("usage text = %q", errOut.String())
	}
}

func TestRunMissingPageFails(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"testdata/missing.md"}, &out, &errOut); code != 1 {
		t.Errorf("run() = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "missing.md") {
		t.Errorf("stderr = %q, want the missing page named", errOut.String())
	}
}

func TestRunPageWithoutBlocksPasses(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"testdata/no-blocks.md"}, &out, &errOut); code != 0 {
		t.Errorf("run() = %d, want 0; stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
}
