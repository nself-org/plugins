package detach

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func aged(t *testing.T, path string, age time.Duration) {
	t.Helper()
	old := time.Now().Add(-age)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
}

func seedStreams(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	dir := Dir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"keep", "gone", "young"} {
		line := `{"schema_version":"1","command":"ci","data":{"type":"queued","seq":1,"run_id":"` + id + `"}}`
		writeLines(t, filepath.Join(dir, id+".ndjson"), line)
	}
	writeLines(t, filepath.Join(dir, "0123abcd.ndjson"), "died before it queued") // a token-named file with no run
	for _, f := range []string{"keep", "gone", "0123abcd"} {
		aged(t, filepath.Join(dir, f+".ndjson"), 3*time.Hour)
	}
	return home
}

func TestSweepRemovesOnlyOldOrphans(t *testing.T) {
	home := seedStreams(t)
	known := func(id string) bool { return id == "keep" || id == "young" }
	dry, err := Sweep(home, known, time.Time{}, true)
	if err != nil || len(dry) != 2 {
		t.Fatalf("dry run lists %v err %v", dry, err)
	}
	if _, err = os.Stat(filepath.Join(Dir(home), "gone.ndjson")); err != nil {
		t.Fatal("a dry run removed a file")
	}
	got, err := Sweep(home, known, time.Time{}, false)
	if err != nil || len(got) != 2 {
		t.Fatalf("removed %v err %v", got, err)
	}
	for _, name := range []string{"keep", "young"} {
		if _, err = os.Stat(filepath.Join(Dir(home), name+".ndjson")); err != nil {
			t.Errorf("%s was removed: %v", name, err)
		}
	}
	for _, name := range []string{"gone", "0123abcd"} {
		if _, err = os.Stat(filepath.Join(Dir(home), name+".ndjson")); !os.IsNotExist(err) {
			t.Errorf("%s survived: %v", name, err)
		}
	}
	if got, err = Sweep(t.TempDir(), known, time.Time{}, false); err != nil || got != nil {
		t.Fatalf("no directory: %v %v", got, err)
	}
}

func TestRemoveNamedRuns(t *testing.T) {
	home := seedStreams(t)
	got, err := Remove(home, []string{"young", "missing", "../escape"})
	if err != nil || len(got) != 1 || got[0] != "young.ndjson" {
		t.Fatalf("removed %v err %v", got, err)
	}
	if _, err = os.Stat(filepath.Join(Dir(home), "keep.ndjson")); err != nil {
		t.Fatal("Remove touched another run")
	}
}
