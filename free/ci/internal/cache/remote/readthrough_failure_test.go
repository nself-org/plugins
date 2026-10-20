package remote

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func TestNodeReadThroughCancellationPreservesCopy(t *testing.T) {
	c, err := NewReadThrough(t.TempDir(), 100)
	if err != nil {
		t.Fatal(err)
	}
	data := "synthetic cache bytes"
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
	f, err := c.Open(context.Background(), "realm", digest, int64(len(data)), func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Open(ctx, "realm", digest, int64(len(data)), nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled verification returned %v", err)
	}
	f, err = c.Open(context.Background(), "realm", digest, int64(len(data)), nil)
	if err != nil {
		t.Fatalf("cancellation discarded intact copy: %v", err)
	}
	f.Close()
}

func TestNodeReadThroughPrunesOrphan(t *testing.T) {
	c, err := NewReadThrough(t.TempDir(), 3)
	if err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(c.dir, ".fetch-orphan")
	if err := os.WriteFile(orphan, []byte("orphan"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.Prune(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("orphan temporary file survived pruning: %v", err)
	}
}

func TestNodeReadThroughReservesFetch(t *testing.T) {
	c, err := NewReadThrough(t.TempDir(), 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{"aa", "bb"} {
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
		f, err := c.Open(context.Background(), "realm", digest, 2, func(_ context.Context, w io.Writer) error {
			if _, err := io.WriteString(w, data); err != nil {
				return err
			}
			var total int64
			err := filepath.WalkDir(c.dir, func(name string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				info, err := d.Info()
				if err == nil {
					total += info.Size()
				}
				return err
			})
			if err == nil && total > c.max {
				t.Fatalf("fetch exceeded node cap: %d > %d", total, c.max)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
}

func TestNodeStageCopyFailureStillShips(t *testing.T) {
	l := nodeSaveLease()
	s, err := NewNodeStage(filepath.Join(t.TempDir(), "stage"), l)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Copies, err = NewReadThrough(t.TempDir(), 100)
	if err != nil {
		t.Fatal(err)
	}
	digest, size, err := s.blobs.Put(context.Background(), s.realm, strings.NewReader("data"))
	if err != nil {
		t.Fatal(err)
	}
	s.refs = append(s.refs, protocol.CacheRef{LeaseID: l.LeaseID, Spec: "go", Key: strings.Repeat("a", 64), Blobs: []protocol.CacheBlob{{Path: "mod", SHA256: digest, Size: size}}})
	// Replace the copy root with a file to make publication fail independently
	// of the valid staged blob and coordinator transfer.
	if err := os.Remove(s.Copies.dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.Copies.dir, nil, 0600); err != nil {
		t.Fatal(err)
	}
	u := &nodeUploader{offsets: make(chan protocol.Offset, 1)}
	if err := s.Ship(context.Background(), u.offsets, u); err != nil {
		t.Fatal(err)
	}
	if u.frames != 3 {
		t.Fatalf("copy I/O failure blocked shared upload: %d frames", u.frames)
	}
}
