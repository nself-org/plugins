//go:build !windows

package hygiene

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sentinelTree makes a directory outside any root that a bad removal would damage.
func sentinelTree(t *testing.T) (dir, file string) {
	t.Helper()
	dir = t.TempDir()
	file = filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(file, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, file
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("%s was damaged: %v", path, err)
	}
}

func TestRemoveConfinedFollowsNoSymlinks(t *testing.T) {
	outside, keep := sentinelTree(t)
	home := t.TempDir() // plays $HOME: content must survive
	if err := os.WriteFile(filepath.Join(home, "profile"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	root := filepath.Join(base, "att1")
	for _, d := range []string{"a/b/c", "tmp"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	links := map[string]string{
		"home-link":      home,
		"a/up":           "../../..",
		"a/b/abs":        outside,
		"a/b/c/rel-out":  filepath.Join("..", "..", "..", "..", filepath.Base(outside)),
		"tmp/file-link":  keep,
		"tmp/dangling":   "/nonexistent/nowhere",
		"tmp/self-loop":  "self-loop",
		"tmp/root-point": root,
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "tmp", "f"), []byte("y"), 0o600); err != nil {
		t.Fatal(err)
	}
	var logs []string
	if err := RemoveConfined(base, "att1", func(f string, a ...any) { logs = append(logs, f) }); err != nil {
		t.Fatalf("remove: %v (logs %v)", err, logs)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("root survived: %v", err)
	}
	mustExist(t, keep)
	mustExist(t, filepath.Join(home, "profile"))
	mustExist(t, base)
}

func TestRemoveConfinedOpensLockedDirectories(t *testing.T) {
	base := t.TempDir()
	locked := filepath.Join(base, "att1", "x", "y")
	if err := os.MkdirAll(locked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "f"), []byte("z"), 0o400); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{locked, filepath.Join(base, "att1", "x"), filepath.Join(base, "att1")} {
		if err := os.Chmod(d, 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := RemoveConfined(base, "att1", nil); err != nil {
		t.Fatalf("a job that chmod-ed its tree to 0 kept it alive: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(base, "att1")); !os.IsNotExist(err) {
		t.Fatal("root survived")
	}
}

func TestRemoveConfinedRefusals(t *testing.T) {
	outside, keep := sentinelTree(t)
	base := t.TempDir()
	var logs []string
	logf := func(f string, a ...any) { logs = append(logs, f) }
	for _, name := range []string{"..", ".", "", "a/b", "../" + filepath.Base(outside), "/etc"} {
		if err := RemoveConfined(base, name, logf); codeOf(err) != CodeLeak {
			t.Errorf("name %q not refused: %v", name, err)
		}
	}
	if len(logs) == 0 {
		t.Error("refusals were not logged")
	}
	// a symlinked base is refused and its target untouched
	link := filepath.Join(t.TempDir(), "base-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(outside, "att1")); err != nil {
		t.Fatal(err)
	}
	if err := RemoveConfined(link, "att1", logf); codeOf(err) != CodeLeak {
		t.Errorf("symlinked base not refused: %v", err)
	}
	mustExist(t, keep)
	// a root slot holding a symlink is unlinked, the target stays
	if err := os.Symlink(outside, filepath.Join(base, "att2")); err != nil {
		t.Fatal(err)
	}
	if err := RemoveConfined(base, "att2", logf); err != nil {
		t.Errorf("symlink in the slot: %v", err)
	}
	mustExist(t, keep)
	if err := RemoveConfined(base, "never-existed", logf); err != nil {
		t.Errorf("missing root must be a no-op: %v", err)
	}
	if err := RemoveConfined(filepath.Join(base, "no-such-base"), "att1", logf); err != nil {
		t.Errorf("missing base must be a no-op: %v", err)
	}
}

// FuzzConfinedRemove builds a tree of files, directories and symlinks (absolute, relative, looping,
// pointing outside) from the fuzz input and checks the removal never touches the outside sentinel
// and never leaves the root behind.
func FuzzConfinedRemove(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	f.Add([]byte("symlink farm \x00\x01\x02\xff\xfe"))
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0x80, 0x80, 0x40, 0x40, 0x20, 0x10})
	f.Fuzz(func(t *testing.T, data []byte) {
		outside, keep := sentinelTree(t)
		base := t.TempDir()
		root := filepath.Join(base, "att")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		dirs := []string{root}
		for i := 0; i+1 < len(data) && i < 120; i += 2 {
			parent := dirs[int(data[i])%len(dirs)]
			name := string(rune('a'+int(data[i+1])%8)) + string(rune('0'+int(data[i])%4))
			path := filepath.Join(parent, name)
			switch data[i+1] >> 5 {
			case 0, 1:
				if os.Mkdir(path, os.FileMode(0o700)) == nil {
					dirs = append(dirs, path)
				}
			case 2:
				// O_EXCL never writes through a symlink the generator made earlier
				if fh, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600); err == nil {
					_, _ = fh.Write(data[:i+1])
					_ = fh.Close()
				}
			case 3:
				_ = os.Symlink(outside, path)
			case 4:
				_ = os.Symlink(keep, path)
			case 5:
				_ = os.Symlink(strings.Repeat("../", int(data[i])%6)+filepath.Base(outside), path)
			case 6:
				_ = os.Symlink(dirs[int(data[i+1])%len(dirs)], path)
			default:
				_ = os.Symlink(name, path)
			}
		}
		_ = RemoveConfined(base, "att", nil)
		if _, err := os.Lstat(root); !os.IsNotExist(err) {
			t.Fatalf("root survived: %v", err)
		}
		b, err := os.ReadFile(keep)
		if err != nil || string(b) != "keep" {
			t.Fatalf("outside sentinel damaged: %v %q", err, b)
		}
	})
}

// TestRemoveConfinedRefusesAnEntrySwappedForASiblingLink swaps the attempt directory for a relative
// symlink to a sibling attempt between inspection and opening: the sibling must survive.
func TestRemoveConfinedRefusesAnEntrySwappedForASiblingLink(t *testing.T) {
	base := t.TempDir()
	for _, d := range []string{"att1", "sibling"} {
		if err := os.Mkdir(filepath.Join(base, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	keep := filepath.Join(base, "sibling", "work.txt")
	if err := os.WriteFile(keep, []byte("live"), 0o600); err != nil {
		t.Fatal(err)
	}
	afterInspect = func() {
		_ = os.Remove(filepath.Join(base, "att1"))
		_ = os.Symlink("sibling", filepath.Join(base, "att1"))
	}
	t.Cleanup(func() { afterInspect = nil })
	err := RemoveConfined(base, "att1", nil)
	if codeOf(err) != CodeLeak {
		t.Fatalf("want a refusal, got %v", err)
	}
	mustExist(t, keep)
}
