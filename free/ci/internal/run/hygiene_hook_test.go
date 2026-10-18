package run

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/hygiene"
)

func newHygieneHook() *hygieneHook { return &hygieneHook{byRun: map[string][]string{}} }

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
	for _, id := range []string{"orphan1", "marked1", "fresh1"} {
		root, err := hygiene.JobRoot(work, id)
		if err != nil {
			t.Fatal(err)
		}
		if id != "fresh1" {
			if err := os.Chtimes(root, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.MkdirAll(filepath.Join(home, "processes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "processes", "marked1.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHygieneHook()
	if err := h.AttemptStart(context.Background(), AttemptInfo{RunID: "r1", AttemptID: "own1"}); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]bool{"orphan1": false, "marked1": true, "fresh1": true} {
		_, err := os.Lstat(filepath.Join(work, id))
		if (err == nil) != want {
			t.Errorf("%s exists=%v want %v", id, err == nil, want)
		}
	}
	if _, err := hygiene.JobRoot(work, "orphan2"); err != nil {
		t.Fatal(err)
	}
	old2 := time.Now().Add(-3 * orphanGrace)
	_ = os.Chtimes(filepath.Join(work, "orphan2"), old2, old2)
	_ = h.AttemptStart(context.Background(), AttemptInfo{RunID: "r1", AttemptID: "own2"})
	if _, err := os.Lstat(filepath.Join(work, "orphan2")); err != nil {
		t.Fatal("the sweep must run once per process")
	}
}
