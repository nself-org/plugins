package update

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSwapPreservesPreviousWithoutHardLinks(t *testing.T) {
	for _, hardLinks := range []bool{true, false} {
		t.Run(map[bool]string{true: "hard link", false: "copy fallback"}[hardLinks], func(t *testing.T) {
			dir := t.TempDir()
			bin, staged := filepath.Join(dir, "agent"), filepath.Join(dir, "staged")
			if err := os.WriteFile(bin, []byte("old"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(staged, []byte("new"), 0600); err != nil {
				t.Fatal(err)
			}
			link := os.Link
			if !hardLinks {
				link = func(string, string) error { return errors.New("hard links unavailable") }
			}
			if err := swap(bin, staged, link); err != nil {
				t.Fatal(err)
			}
			for _, f := range []struct{ path, want string }{{bin, "new"}, {bin + PrevSuffix, "old"}} {
				got, err := os.ReadFile(f.path)
				if err != nil || string(got) != f.want {
					t.Fatalf("%s: %q, %v", f.path, got, err)
				}
				info, err := os.Stat(f.path)
				if err != nil || info.Mode().Perm() != 0755 {
					t.Fatalf("mode: %v, %v", info, err)
				}
			}
		})
	}
}
