package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
