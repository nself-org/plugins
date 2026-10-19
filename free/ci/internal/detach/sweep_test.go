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
		line := `{"schema_version":"1","command":"ci","data":{"type":"queued","schema":"ci.events/v1","seq":1,"run_id":"` + id + `"}}`
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

func TestRunIDOfReadsOnlyTheStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.ndjson")
	big := make([]byte, 0, 3<<20)
	big = append(big, []byte(`{"schema_version":"1","command":"ci","data":{"type":"queued","schema":"ci.events/v1","seq":1,"run_id":"r-big"}}`+"\n")...)
	for len(big) < 3<<20 {
		big = append(big, []byte("x\n")...)
	}
	if err := os.WriteFile(path, big, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := RunIDOf(path); got != "r-big" {
		t.Fatalf("RunIDOf = %q", got)
	}
	r := &Reader{Path: path}
	defer func() { _ = r.Close() }()
	lines, err := r.Next()
	if err != nil || len(lines) == 0 || len(lines) >= len(big)/2 { // one call reads at most maxChunk bytes
		t.Fatalf("lines %d err %v", len(lines), err)
	}
}

func TestPIDSidecarAndSweep(t *testing.T) {
	home := seedStreams(t)
	stream := filepath.Join(Dir(home), "gone.ndjson")
	if WriterAlive(stream) {
		t.Fatal("no pid recorded must read as not alive")
	}
	if err := WritePID(stream, os.Getpid()); err != nil {
		t.Fatal(err)
	}
	if ReadPID(stream) != os.Getpid() || !WriterAlive(stream) {
		t.Fatalf("pid %d alive %v", ReadPID(stream), WriterAlive(stream))
	}
	if st, err := os.Stat(pidPath(stream)); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("pid file: %v %v", st, err)
	}
	if _, err := Sweep(home, func(id string) bool { return id != "gone" }, time.Time{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pidPath(stream)); !os.IsNotExist(err) {
		t.Fatalf("the pid file outlived its stream: %v", err)
	}
	if err := os.WriteFile(pidPath(stream), []byte("2147483646\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if WriterAlive(stream) {
		t.Fatal("a pid that does not exist read as alive")
	}
}
