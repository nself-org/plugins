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
	"time"

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
	if _, err := os.Stat(orphan); err != nil {
		t.Fatalf("a temporary file another lease is still writing was pruned: %v", err)
	}
	old := time.Now().Add(-2 * fetchTempGrace)
	if err := os.Chtimes(orphan, old, old); err != nil {
		t.Fatal(err)
	}
	if err := c.Prune(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("orphan temporary file survived pruning: %v", err)
	}
}

// A prune that runs while another lease is mid-fetch must not delete the transfer's temporary file.
func TestNodeReadThroughPruneSparesAFetchInFlight(t *testing.T) {
	c, err := NewReadThrough(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("a blob that arrives while another lease prunes")
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	f, err := c.Open(context.Background(), "realm", digest, int64(len(data)), func(_ context.Context, w io.Writer) error {
		if _, err := w.Write(data[:10]); err != nil {
			return err
		}
		if err := c.Prune(); err != nil { // the other lease starts
			return err
		}
		_, err := w.Write(data[10:])
		return err
	})
	if err != nil {
		t.Fatalf("a prune during the fetch spoiled it: %v", err)
	}
	_ = f.Close()
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

// A corrupt copy that was verified is removed; a fresh copy another lease renamed over it since is not.
func TestRemoveIfSameSparesAReplacedCopy(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "copy")
	if err := os.WriteFile(name, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	seen, err := os.Lstat(name)
	if err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(dir, "fresh")
	if err = os.WriteFile(fresh, []byte("verified"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(fresh, name); err != nil {
		t.Fatal(err)
	}
	removeIfSame(name, seen)
	if got, err := os.ReadFile(name); err != nil || string(got) != "verified" {
		t.Fatalf("the fresh copy was removed: %q %v", got, err)
	}
	now, _ := os.Lstat(name)
	removeIfSame(name, now)
	if _, err = os.Stat(name); !os.IsNotExist(err) {
		t.Fatal("the verified-corrupt copy was kept")
	}
}

// Two fetches that each fit the cap alone but not together: the second finds no room instead of overrunning the cap.
func TestNodeReadThroughCountsConcurrentFetchReservations(t *testing.T) {
	c, err := NewReadThrough(t.TempDir(), 10)
	if err != nil {
		t.Fatal(err)
	}
	d1, d2 := []byte("0123456789"), []byte("abcdefghij")
	dig := func(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		f, err := c.Open(context.Background(), "realm", dig(d1), 10, func(_ context.Context, w io.Writer) error {
			close(started)
			<-release // the first fetch has claimed 10 B and written none
			_, err := w.Write(d1)
			return err
		})
		if f != nil {
			_ = f.Close()
		}
		done <- err
	}()
	<-started
	_, err = c.Open(context.Background(), "realm", dig(d2), 10, func(_ context.Context, w io.Writer) error { _, err := w.Write(d2); return err })
	if err == nil || !strings.Contains(err.Error(), "E711") {
		t.Fatalf("second fetch under a full reservation: %v", err)
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatalf("first fetch: %v", err)
	}
}

// A fetch returns the descriptor that was hashed: pruning the published name right after the rename cannot take the bytes away.
func TestNodeReadThroughOpenSurvivesAPruneAfterPublish(t *testing.T) {
	c, err := NewReadThrough(t.TempDir(), 10)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("0123456789")
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	f, err := c.Open(context.Background(), "realm", digest, 10, func(_ context.Context, w io.Writer) error { _, err := w.Write(data); return err })
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	name, _ := c.name("realm", digest)
	if err = os.Remove(name); err != nil { // another lease evicted it
		t.Fatal(err)
	}
	got, err := io.ReadAll(f)
	if err != nil || string(got) != string(data) {
		t.Fatalf("read %q %v after the name was pruned", got, err)
	}
}
