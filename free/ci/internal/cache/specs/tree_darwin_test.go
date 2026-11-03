//go:build darwin

package specs

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCloneTreeWarmHit(t *testing.T) {
	e := newEnv(t)
	e.eng.Trees = filepath.Join(e.home, "cache-trees")
	saved(t, e, "warm-save", ownerFacts).Cleanup()
	treeRestore(t, e, "warm-first").Cleanup()
	before := map[string]os.FileInfo{}
	err := filepath.Walk(e.eng.Trees, func(path string, info os.FileInfo, err error) error {
		if err == nil {
			before[path] = info
		}
		return err
	})
	if err != nil || len(before) < 3 {
		t.Fatalf("first restore made no trees: %d paths, %v", len(before), err)
	}
	var unpacked atomic.Int64
	afterFileStart = func() { unpacked.Add(1) }
	t.Cleanup(func() { afterFileStart = nil })
	treeRestore(t, e, "warm-second").Cleanup()
	if got := unpacked.Load(); got != 0 {
		t.Fatalf("warm restore unpacked %d files", got)
	}
	seen := 0
	err = filepath.Walk(e.eng.Trees, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		old, ok := before[path]
		if !ok || !os.SameFile(old, info) {
			t.Errorf("warm restore added or replaced tree path %s", path)
		}
		seen++
		return nil
	})
	if err != nil || seen != len(before) {
		t.Fatalf("tree paths changed: before %d, after %d, %v", len(before), seen, err)
	}
}

func TestCloneTreeIsolation(t *testing.T) {
	src := filepath.Join(t.TempDir(), "tree")
	putFile(t, filepath.Join(src, "nested", "file"), "original")
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	if err := cloneTree(src, first); err != nil {
		t.Fatal(err)
	}
	if err := cloneTree(src, second); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(first, "nested", "file"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{src, second} {
		if got := getFile(t, filepath.Join(dir, "nested", "file")); got != "original" {
			t.Fatalf("writing first clone changed %s: %q", dir, got)
		}
	}
	if got := getFile(t, filepath.Join(first, "nested", "file")); got != "changed" {
		t.Fatalf("first clone: %q", got)
	}
}

func TestCloneTreeWarmBlobStillVerified(t *testing.T) {
	e := newEnv(t)
	e.eng.Trees = filepath.Join(e.home, "cache-trees")
	saved(t, e, "verified-save", ownerFacts).Cleanup()
	treeRestore(t, e, "verified-warm").Cleanup()
	trees, err := os.ReadDir(e.eng.Trees)
	if err != nil || len(trees) != 2 {
		t.Fatalf("warm trees: %v, %v", trees, err)
	}
	flipped := 0
	err = filepath.WalkDir(filepath.Join(e.home, "blobs"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if i := bytes.Index(raw, []byte("compiled")); i >= 0 {
			raw[i] = 'X'
			flipped++
			return os.WriteFile(path, raw, 0o600)
		}
		return nil
	})
	if err != nil || flipped != 1 {
		t.Fatalf("corrupt blob: flipped %d, %v", flipped, err)
	}
	e.running("verified-refused", ownerFacts)
	s := e.begin(e.input("verified-refused", ownerFacts))
	if got := strings.Join(resultsOf(e.uses("verified-refused")), ","); got != "go:refused,go:miss" {
		t.Fatalf("warm corrupt blob restore: %s", got)
	}
	for _, name := range []string{"build", "mod"} {
		ents, err := os.ReadDir(s.dir(name))
		if err != nil || len(ents) != 0 {
			t.Fatalf("refused %s holds entries: %v, %v", name, ents, err)
		}
	}
}
