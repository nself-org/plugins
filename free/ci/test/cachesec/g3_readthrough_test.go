//go:build cachesec

package cachesec

// Purpose: C15-NODE-01, node-side TM-15 behaviour of cache/remote.ReadThrough: a corrupted copy is deleted and never
// served, and a fetch that delivers other bytes than the digest names is refused and publishes nothing. The node holds
// no ledger, so production appends no audit row (debt D-1452).
// Inputs: a temp read-through dir. Outputs: the registered case C15-NODE-01 (carrier agent: the copy lives on a node).
// Constraints: production code only. ReadThrough returns an untyped error ("E714 digest_mismatch: ..."), so the code
// and reason key are derived from that error text prefix, the only form production offers.

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

func init() {
	register("C15-NODE-01", g3NodeReadThrough)
}

func g3NodeReadThrough(t *testing.T) Observed {
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
	if f != nil || err == nil {
		t.Fatalf("a corrupting fetch was published: file=%v err=%v", f != nil, err)
	}
	fields := strings.Fields(err.Error()) // "E714 digest_mismatch: node cache copy failed verification"
	if len(fields) < 2 || !strings.HasPrefix(fields[0], "E") {
		t.Fatalf("the refusal carries no fabric code prefix: %v", err)
	}
	refusal := Observed{Outcome: Refused, Code: fields[0], Reason: strings.TrimSuffix(fields[1], ":")}
	if _, err = rt.Open(ctx, "realm-b", sum, int64(len(data)), nil); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the refused fetch left a copy behind: %v", err)
	}
	var landed int64
	_ = filepath.Walk(dir, func(_ string, fi os.FileInfo, _ error) error {
		if fi != nil && fi.Mode().IsRegular() {
			landed += fi.Size()
		}
		return nil
	})
	refusal.Audit, refusal.Landed = "none", landed
	return refusal
}
