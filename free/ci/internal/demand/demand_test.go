package demand

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeClock struct {
	now time.Time
}

func (f *fakeClock) Now() time.Time { return f.now }

func TestDemandAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	fc := &fakeClock{now: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)}
	cfg := WriterConfig{Dir: dir, Host: "test-host", Clock: fc}
	doc := Document{
		Queued:   3,
		Held:     1,
		Running:  2,
		Ready:    true,
		ReadyWhy: []string{},
		Watcher: Watcher{
			Alive:       true,
			LastPass:    fc.now.Format(time.RFC3339),
			IntervalSec: 30,
		},
	}
	if err := Write(cfg, doc); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	target := filepath.Join(dir, fileName)
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if perm := info.Mode().Perm(); perm != fileMode {
		t.Errorf("file mode = %o, want %o", perm, fileMode)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	var got Document
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.TS != fc.now.Format(time.RFC3339) {
		t.Errorf("TS = %q, want %q", got.TS, fc.now.Format(time.RFC3339))
	}
	if got.Host != "test-host" {
		t.Errorf("Host = %q, want %q", got.Host, "test-host")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".demand-") && strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestDemandWhenNotReady(t *testing.T) {
	dir := t.TempDir()
	fc := &fakeClock{now: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)}
	cfg := WriterConfig{Dir: dir, Host: "not-ready-host", Clock: fc}
	doc := Document{
		Queued:   5,
		Held:     0,
		Running:  0,
		Ready:    false,
		ReadyWhy: []string{"provider unavailable"},
		Watcher: Watcher{
			Alive:       true,
			LastPass:    fc.now.Format(time.RFC3339),
			IntervalSec: 30,
		},
	}
	if err := Write(cfg, doc); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	snap, err := Read(ReadConfig{Dir: dir, Clock: fc})
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if snap.Doc.Ready {
		t.Error("expected Ready=false")
	}
	if len(snap.Doc.ReadyWhy) != 1 || snap.Doc.ReadyWhy[0] != "provider unavailable" {
		t.Errorf("ReadyWhy = %v, want [provider unavailable]", snap.Doc.ReadyWhy)
	}
	if snap.Doc.Queued != 5 {
		t.Errorf("Queued = %d, want 5", snap.Doc.Queued)
	}
}

func TestDemandReadAge(t *testing.T) {
	dir := t.TempDir()
	writeTime := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	readTime := writeTime.Add(45 * time.Second)
	wc := WriterConfig{Dir: dir, Host: "age-host", Clock: &fakeClock{now: writeTime}}
	doc := Document{
		Queued:   0,
		Held:     0,
		Running:  0,
		Ready:    true,
		ReadyWhy: []string{},
		Watcher: Watcher{
			Alive:       true,
			LastPass:    writeTime.Format(time.RFC3339),
			IntervalSec: 30,
		},
	}
	if err := Write(wc, doc); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	rc := ReadConfig{Dir: dir, Clock: &fakeClock{now: readTime}}
	snap, err := Read(rc)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if snap.Age != 45*time.Second {
		t.Errorf("Age = %v, want 45s", snap.Age)
	}
	if !snap.Doc.Watcher.Alive {
		t.Error("expected Watcher.Alive=true")
	}
	missingSnap, err := Read(ReadConfig{Dir: t.TempDir(), Clock: &fakeClock{now: readTime}})
	if err != nil {
		t.Fatalf("Read missing failed: %v", err)
	}
	if missingSnap.Doc.Watcher.Alive {
		t.Error("expected Alive=false for missing file")
	}
}

func TestDemandNoSensitiveFields(t *testing.T) {
	dir := t.TempDir()
	fc := &fakeClock{now: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)}
	cfg := WriterConfig{Dir: dir, Host: "safe-host", Clock: fc}
	doc := Document{
		Queued:   0,
		Held:     0,
		Running:  0,
		Ready:    true,
		ReadyWhy: []string{},
		Watcher: Watcher{
			Alive:       true,
			LastPass:    fc.now.Format(time.RFC3339),
			IntervalSec: 30,
		},
	}
	if err := Write(cfg, doc); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	content := string(data)
	for _, forbidden := range []string{"token", "secret", "password", "/home/", "/var/", "git@"} {
		if strings.Contains(strings.ToLower(content), forbidden) {
			t.Errorf("document contains sensitive pattern %q", forbidden)
		}
	}
}
