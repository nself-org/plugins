package exec

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPidsWatchReadsCountersIndependently(t *testing.T) {
	for _, missing := range []string{"pids.events", "memory.events"} {
		t.Run(missing, func(t *testing.T) {
			dir := fixtureCgroup(t, t.TempDir(), "container", "100")
			for file, body := range map[string]string{"pids.events": "max 4\n", "memory.events": "max 7\noom_kill 2\n"} {
				if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Remove(filepath.Join(dir, missing)); err != nil {
				t.Fatal(err)
			}
			w := &pidsWatch{r: pidsReading{Path: dir}}
			w.sample(context.Background())
			r := w.snapshot()
			if missing == "pids.events" {
				if r.Reads != 0 || r.PidsError == "" || r.MemReads != 1 || r.MemMax != 7 || r.OOMKill != 2 || r.MemError != "" {
					t.Fatalf("memory evidence lost when pids read failed: %+v", r)
				}
			} else if r.MemReads != 0 || r.MemError == "" || r.Reads != 1 || r.Max != 4 || r.PidsError != "" {
				t.Fatalf("pids evidence lost when memory read failed: %+v", r)
			}
			if !strings.Contains(r.String(), missing) || !strings.Contains(r.String(), "no such file") {
				t.Fatalf("read error missing from diagnostics: %s", r.String())
			}
		})
	}
}

func TestPidsWatchStopReadsFinalCounters(t *testing.T) {
	dir := fixtureCgroup(t, t.TempDir(), "container", "100")
	w := &pidsWatch{cancel: func() {}, r: pidsReading{Path: dir}}
	w.sample(context.Background())
	if err := os.WriteFile(filepath.Join(dir, "pids.events"), []byte("max 8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := w.stop()
	if r.Max != 8 || r.Reads != 2 {
		t.Fatalf("stop did not preserve final counters: %+v", r)
	}
	if r.MemReads != 0 || r.MemError == "" {
		t.Fatalf("missing memory evidence became a successful zero read: %+v", r)
	}
	if again := w.stop(); again != r {
		t.Fatalf("stop repeated sampling after evidence preservation: got %+v want %+v", again, r)
	}
}

func TestPidsWatchStopDiscoversExitedContainer(t *testing.T) {
	root, proc := cgroupRoot, procRoot
	cgroupRoot, procRoot = t.TempDir(), t.TempDir()
	t.Cleanup(func() { cgroupRoot, procRoot = root, proc })
	dir := fixtureCgroup(t, cgroupRoot, "docker/abc", "100")
	if err := os.WriteFile(filepath.Join(dir, "memory.events"), []byte("max 1\noom_kill 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "pids.events")); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "runtime")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf 'abc 0\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	w := &pidsWatch{cancel: func() {}, binary: binary, name: "container"}
	r := w.stop()
	if r.Path != dir || r.MemReads != 1 || r.OOMKill != 1 || r.Reads != 0 || r.PidsError == "" {
		t.Fatalf("final discovery lost exited container memory evidence: %+v", r)
	}
}

func TestParsePidsMax(t *testing.T) {
	for body, want := range map[string]uint64{"max 7\n": 7, "max 0\n": 0, "": 0, "other 5\nmax 3\n": 3, "max x\n": 0} {
		if got := parsePidsMax(body); got != want {
			t.Errorf("%q: %d want %d", body, got, want)
		}
	}
}

func fixtureCgroup(t *testing.T, root, rel, limit string) string {
	t.Helper()
	dir := filepath.Join(root, rel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for f, v := range map[string]string{"pids.events": "max 0\n", "pids.max": limit + "\n"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(v), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestPickCgroupPrefersTheInitProcessCgroupWithANumericLimit(t *testing.T) {
	root, proc := t.TempDir(), t.TempDir()
	want := fixtureCgroup(t, root, "system.slice/docker-abc.scope", "100")
	fixtureCgroup(t, root, "system.slice", "max") // a parent slice must never be taken
	_ = os.MkdirAll(filepath.Join(proc, "42"), 0o755)
	_ = os.WriteFile(filepath.Join(proc, "42", "cgroup"), []byte("0::/system.slice\n"), 0o644)
	got, problem := pickCgroup(root, proc, "abc", "42")
	if got != want || problem != "" {
		t.Fatalf("got %q (%s) want %q", got, problem, want)
	}
}

func TestPickCgroupNamesWhatItTriedWhenNothingFits(t *testing.T) {
	got, problem := pickCgroup(t.TempDir(), t.TempDir(), "abc", "42")
	if got != "" || !strings.Contains(problem, "docker-abc.scope") {
		t.Fatalf("%q %q", got, problem)
	}
}

func TestPidsReadingHitAndText(t *testing.T) {
	if (pidsReading{}).Hit() || !(pidsReading{Max: 2}).Hit() {
		t.Fatal("Hit follows the counter")
	}
	s := pidsReading{Max: 3, Path: "/c", Limit: "100", Reads: 9}.String()
	if !strings.Contains(s, "max=3") || !strings.Contains(s, "pids.max=100") {
		t.Fatal(s)
	}
	if !strings.Contains((pidsReading{Problem: "p"}).String(), "(p)") {
		t.Fatal("a miss says why")
	}
	if (*pidsWatch)(nil).stop().Hit() {
		t.Fatal("nil watch reads nothing")
	}
}
