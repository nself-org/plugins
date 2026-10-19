package records_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

// FuzzCollect throws declaration strings at Normalize and Collect over a fixed
// workspace that holds a symlink out, a hard link out, a FIFO and ordinary
// files. Invariants: no panic; a success only names files inside the
// workspace; nothing is ever written outside the store tree.
func FuzzCollect(f *testing.F) {
	for _, s := range []string{"a.txt", "../../etc/passwd", "/etc/passwd", "d/**", "**", "*", "d/../../x", "link", "hard", "pipe", "d/[a-", ".git/config", "d//e", "a\x00b", "d/e.txt", "..", ".", "./d", "d/", "ä/ö", "A.TXT", "a\\b", "C:\\x", "d/*/x"} {
		f.Add(s)
	}
	home := f.TempDir()
	ws := filepath.Join(f.TempDir(), "ws")
	outside := filepath.Join(f.TempDir(), "outside")
	for p, c := range map[string]string{filepath.Join(ws, "a.txt"): "a", filepath.Join(ws, "d", "e.txt"): "e", filepath.Join(ws, "d", "sub", "x"): "x", outside: "host"} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			f.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			f.Fatal(err)
		}
	}
	_ = os.Symlink("../../../outside", filepath.Join(ws, "link"))
	_ = os.Link(outside, filepath.Join(ws, "hard"))
	blobs, err := cas.New(home)
	if err != nil {
		f.Fatal(err)
	}
	c := records.Collector{Blobs: blobs, Scanner: redact.New(), Policy: records.Policy{MaxFile: 1 << 20, MaxTotal: 1 << 24, StorePath: home,
		Statfs: func(string) (uint64, uint64, error) { return 1 << 40, 1 << 41, nil }}}
	f.Fuzz(func(t *testing.T, path string) {
		ds, err := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{{Path: path}}}, false, nil)
		if err != nil {
			return
		}
		staged, err := c.Collect(context.Background(), "proj", ws, ds)
		if err != nil {
			return
		}
		for _, s := range staged {
			if strings.HasPrefix(s.Rel, "/") || strings.Contains("/"+s.Rel+"/", "/../") || s.Rel == "link" || s.Rel == "hard" {
				t.Fatalf("collected %q for %q", s.Rel, path)
			}
		}
		if b, _ := os.ReadFile(outside); string(b) != "host" {
			t.Fatal("outside file changed")
		}
	})
}
