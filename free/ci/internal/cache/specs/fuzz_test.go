package specs

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FuzzRestoreArchive: whatever bytes a blob holds, Unpack neither panics nor
// writes outside the attempt dir.
func FuzzRestoreArchive(f *testing.F) {
	for _, ents := range [][]tent{
		{{name: "a/b.txt", body: "x"}},
		{{name: "../x", body: "evil"}},
		{{name: "/abs", body: "evil"}},
		{{name: "l", typ: tar.TypeSymlink, link: "../.."}, {name: "l/x", body: "evil"}},
		{{name: "h", typ: tar.TypeLink, link: "../h"}},
		{{name: "big", size: 4 << 20}},
		{{name: "d/", typ: tar.TypeDir, mode: 0o555}, {name: "d/f", body: "ok"}},
	} {
		f.Add(tarGz(f, ents...))
	}
	f.Add([]byte{})
	f.Add([]byte("\x1f\x8b\x08\x00garbage"))
	f.Fuzz(func(t *testing.T, data []byte) {
		parent := t.TempDir()
		dir := filepath.Join(parent, "dir")
		_, _ = Unpack(bytes.NewReader(data), dir, cacheLimits())
		defer func() { _ = RemoveTree(dir) }()
		ents, err := os.ReadDir(parent)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range ents {
			if e.Name() != "dir" {
				t.Fatalf("Unpack wrote %q next to the attempt dir", e.Name())
			}
		}
		_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err == nil && d.Type()&os.ModeSymlink != 0 {
				if tgt, _ := os.Readlink(p); strings.HasPrefix(tgt, "/") {
					t.Fatalf("absolute symlink %s -> %s", p, tgt)
				}
			}
			return nil
		})
	})
}
