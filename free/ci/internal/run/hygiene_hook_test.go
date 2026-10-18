package run

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/hygiene"
)

func newHygieneHook() *hygieneHook { return &hygieneHook{byRun: map[string][]seenAttempt{}} }

func TestHygieneHookIsRegistered(t *testing.T) {
	var fin, start bool
	for _, f := range Registered.Finalizers {
		fin = fin || f.Name() == hygieneHooks.Name()
	}
	for _, s := range Registered.AttemptStarts {
		start = start || s == AttemptStartHook(hygieneHooks)
	}
	if !fin || !start {
		t.Fatalf("hygiene hook registered: finalizer=%v attempt-start=%v", fin, start)
	}
}

func TestHygieneFinalizerCleansEveryAttemptOfTheRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	h := newHygieneHook()
	for _, id := range []string{"a1", "a2"} {
		root, err := hygiene.JobRoot(filepath.Join(home, "work"), id)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "left"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// a1 was seen by AttemptStart (an earlier retry), a2 only by the job outcome
	if err := h.AttemptStart(context.Background(), AttemptInfo{RunID: "r1", AttemptID: "a1"}); err != nil {
		t.Fatal(err)
	}
	if err := h.Finalize(context.Background(), RunInfo{RunID: "r1", Jobs: []JobOutcome{{AttemptID: "a2"}, {}}}); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(filepath.Join(home, "work")); len(entries) != 0 {
		t.Fatalf("roots survived: %v", entries)
	}
}

func TestHygieneFinalizerDrainsOnLeak(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the directory permission that makes the leak")
	}
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	work := filepath.Join(home, "work")
	if _, err := hygiene.JobRoot(work, "a1"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(work, 0o500); err != nil { // the root cannot be unlinked from a read-only parent
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(work, 0o700) })
	h := newHygieneHook()
	var drained string
	h.drain = func(reason string) { drained = reason }
	err := h.Finalize(context.Background(), RunInfo{RunID: "r1", Jobs: []JobOutcome{{AttemptID: "a1"}}})
	if err == nil || CodeOf(err) != hygiene.CodeLeak {
		t.Fatalf("want E681, got %v", err)
	}
	if !strings.Contains(drained, "a1") {
		t.Fatalf("drain callback got %q", drained)
	}
}

func TestHygieneAttemptStartSweepsOrphansOnceAndSparesLiveWork(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	work := filepath.Join(home, "work")
	old := time.Now().Add(-3 * orphanGrace)
	for _, id := range []string{"orphan.a1", "marked.a1", "fresh.a1"} {
		root, err := hygiene.JobRoot(work, id)
		if err != nil {
			t.Fatal(err)
		}
		if id != "fresh.a1" {
			if err := os.Chtimes(root, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, name := range []string{"nselfci-1a2b3c4d", "nselfci-1a2b3c4d.lock"} { // the services package's, old
		if err := os.WriteFile(filepath.Join(work, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		_ = os.Chtimes(filepath.Join(work, name), old, old)
	}
	if err := os.MkdirAll(filepath.Join(home, "processes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "processes", "marked.a1.json"), []byte(fmt.Sprintf(`{"pid":%d}`, os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHygieneHook()
	if err := h.AttemptStart(context.Background(), AttemptInfo{RunID: "r1", AttemptID: "own1"}); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]bool{"orphan.a1": false, "marked.a1": true, "fresh.a1": true, "nselfci-1a2b3c4d": true, "nselfci-1a2b3c4d.lock": true} {
		_, err := os.Lstat(filepath.Join(work, id))
		if (err == nil) != want {
			t.Errorf("%s exists=%v want %v", id, err == nil, want)
		}
	}
	if _, err := hygiene.JobRoot(work, "orphan.a2"); err != nil {
		t.Fatal(err)
	}
	old2 := time.Now().Add(-3 * orphanGrace)
	_ = os.Chtimes(filepath.Join(work, "orphan.a2"), old2, old2)
	_ = h.AttemptStart(context.Background(), AttemptInfo{RunID: "r1", AttemptID: "own2"})
	if _, err := os.Lstat(filepath.Join(work, "orphan.a2")); err != nil {
		t.Fatal("the sweep must run once per process")
	}
}

func TestHygieneHookUsesTheRunsOwnHome(t *testing.T) {
	envHome, runHome := t.TempDir(), t.TempDir()
	t.Setenv("NSELF_CI_HOME", envHome)
	root, err := hygiene.JobRoot(filepath.Join(runHome, "work"), "a1")
	if err != nil {
		t.Fatal(err)
	}
	h := newHygieneHook()
	if err := h.AttemptStart(context.Background(), AttemptInfo{RunID: "r1", AttemptID: "a1", Home: runHome}); err != nil {
		t.Fatal(err)
	}
	if err := h.Finalize(context.Background(), RunInfo{RunID: "r1", Jobs: []JobOutcome{{AttemptID: "a1"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatal("the root under Options.Home was not cleaned")
	}
}

func TestMarkerLiveRequiresAReadableMarkerOfALiveProcess(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if !markerLive(write("alive.json", fmt.Sprintf(`{"pid":%d}`, os.Getpid()))) {
		t.Error("a marker of a running process must protect its attempt")
	}
	for name, body := range map[string]string{"corrupt.json": "not json", "zero.json": `{"pid":0}`, "dead.json": `{"pid":2147483000}`} {
		if markerLive(write(name, body)) {
			t.Errorf("%s protects an attempt it should not", name)
		}
	}
	if markerLive(filepath.Join(dir, "missing.json")) {
		t.Error("a missing marker protects an attempt")
	}
}

func TestHygieneHookSkipsARelocatedWorkDirectory(t *testing.T) {
	home, elsewhere := t.TempDir(), t.TempDir()
	if err := os.Symlink(elsewhere, filepath.Join(home, "work")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NSELF_CI_HOME", home)
	h := newHygieneHook()
	if err := h.AttemptStart(context.Background(), AttemptInfo{RunID: "r1", AttemptID: "a1.a1", Home: home}); err != nil {
		t.Fatal(err)
	}
	if err := h.Finalize(context.Background(), RunInfo{RunID: "r1", Jobs: []JobOutcome{{AttemptID: "a1.a1"}}}); err != nil {
		t.Fatalf("an operator-relocated work directory must not drain the node: %v", err)
	}
}
