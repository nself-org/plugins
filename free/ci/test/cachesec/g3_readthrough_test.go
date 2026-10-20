//go:build cachesec

package cachesec

// Purpose: node-side TM-15 behaviour of cache/remote.ReadThrough (FINDING-g3-2: the refusal is correct but node-side,
// so it has no audit row and no matrix row): a corrupted copy is deleted and never served, and a fetch that delivers
// other bytes than the digest names is refused and publishes nothing.
// Inputs: a temp read-through dir. Outputs: pass or fail. Constraints: production code only.

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cacheremote "github.com/nself-org/plugins/free/ci/internal/cache/remote"
)

func TestG3NodeReadThroughRefusesCorruption(t *testing.T) {
	dir := t.TempDir()
	rt, err := cacheremote.NewReadThrough(dir, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("SYNTHETIC read-through payload")
	sum := g3Sum(data)
	ctx := context.Background()
	f, err := rt.Open(ctx, "realm-a", sum, int64(len(data)), func(_ context.Context, w io.Writer) error { _, err := w.Write(data); return err })
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	var copyPath string
	_ = filepath.Walk(dir, func(p string, fi os.FileInfo, _ error) error {
		if fi != nil && fi.Mode().IsRegular() && filepath.Base(p) == sum {
			copyPath = p
		}
		return nil
	})
	if copyPath == "" {
		t.Fatal("no copy found")
	}
	if err := os.WriteFile(copyPath, []byte("SYNTHETIC corrupted!!payload!!"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err = rt.Open(ctx, "realm-a", sum, int64(len(data)), nil)
	if f != nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a corrupted node copy was served or not reported missing: file=%v err=%v", f != nil, err)
	}
	if _, statErr := os.Stat(copyPath); !os.IsNotExist(statErr) {
		t.Fatal("the corrupted copy was left on disk")
	}
	f, err = rt.Open(ctx, "realm-b", sum, int64(len(data)), func(_ context.Context, w io.Writer) error {
		_, err := w.Write([]byte(strings.ToUpper(string(data))))
		return err
	})
	if f != nil || err == nil || !strings.Contains(err.Error(), "E714 digest_mismatch") {
		t.Fatalf("a corrupting fetch was published: file=%v err=%v", f != nil, err)
	}
	if _, err = rt.Open(ctx, "realm-b", sum, int64(len(data)), nil); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the refused fetch left a copy behind: %v", err)
	}
}
