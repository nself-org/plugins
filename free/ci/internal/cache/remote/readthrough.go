package remote

// Purpose: keep bounded, realm-isolated node copies of coordinator cache blobs.
// Inputs: an agent-owned directory, digest, size and optional transfer callback.
// Outputs: a pinned file, rewound after SHA256 verification on every use.
// Constraints: corrupted copies are deleted; a fetched blob is published only
// after verification; eviction runs at lease start and before publication.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
)

const DefaultNodeCacheMaxBytes int64 = 5 << 30

type ReadThrough struct {
	dir string
	max int64
}

// Instances share the lock: concurrent leases use the same on-disk cap.
var readThroughMu sync.Mutex

var errCopyMismatch = transfer.Failf(transfer.ReasonDigest, "node cache copy failed verification")

func NewReadThrough(dir string, maxBytes int64) (*ReadThrough, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultNodeCacheMaxBytes
		if raw := os.Getenv("NSELF_CI_NODE_CACHE_MAX_BYTES"); raw != "" {
			n, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("invalid node cache byte cap")
			}
			maxBytes = n
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &ReadThrough{dir: dir, max: maxBytes}, nil
}

func (c *ReadThrough) name(realm, digest string) (string, error) {
	b, err := hex.DecodeString(digest)
	if err != nil || len(b) != sha256.Size || hex.EncodeToString(b) != digest || realm == "" {
		return "", fmt.Errorf("invalid cache realm or digest")
	}
	r := sha256.Sum256([]byte(realm))
	return filepath.Join(c.dir, hex.EncodeToString(r[:]), digest), nil
}

// Open verifies an existing copy before returning it, or fetches and verifies a
// replacement. The caller owns the returned file. A nil fetch permits hits only.
func (c *ReadThrough) Open(ctx context.Context, realm, digest string, size int64, fetch func(context.Context, io.Writer) error) (*os.File, error) {
	readThroughMu.Lock()
	defer readThroughMu.Unlock()
	name, err := c.name(realm, digest)
	if err != nil {
		return nil, err
	}
	if size < 0 || size > c.max {
		return nil, fmt.Errorf("E711 size_cap: node cache blob exceeds cap")
	}
	if f, err := verifiedFile(ctx, name, digest, size); err == nil {
		now := time.Now()
		_ = os.Chtimes(name, now, now)
		return f, nil
	} else if errors.Is(err, errCopyMismatch) {
		_ = os.Remove(name)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if fetch == nil {
		return nil, os.ErrNotExist
	}
	if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		return nil, err
	}
	if err := c.prune(size); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(filepath.Dir(name), ".fetch-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close(); _ = os.Remove(f.Name()) }()
	w := &boundedCopy{ctx: ctx, w: f, remaining: size}
	if err := fetch(ctx, w); err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	check, err := verifiedFile(ctx, f.Name(), digest, size)
	if err != nil {
		return nil, err
	}
	_ = check.Close()
	if err := os.Rename(f.Name(), name); err != nil {
		return nil, err
	}
	return verifiedFile(ctx, name, digest, size)
}

func verifiedFile(ctx context.Context, name, digest string, size int64) (*os.File, error) {
	info, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("cache copy unavailable")
	}
	f, err := os.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	got, err := f.Stat()
	if err != nil || !got.Mode().IsRegular() || !os.SameFile(info, got) {
		_ = f.Close()
		return nil, fmt.Errorf("cache copy changed while opening")
	}
	h := sha256.New()
	n, err := io.Copy(h, &contextReader{ctx, f})
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if n != size || hex.EncodeToString(h.Sum(nil)) != digest {
		_ = f.Close()
		return nil, errCopyMismatch
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// Prune applies the configured global byte cap across all realm copies.
func (c *ReadThrough) Prune() error {
	readThroughMu.Lock()
	defer readThroughMu.Unlock()
	return c.prune(0)
}

func (c *ReadThrough) prune(reserve int64) error {
	type copyInfo struct {
		name string
		size int64
		at   time.Time
	}
	var copies []copyInfo
	var total int64
	err := filepath.WalkDir(c.dir, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".fetch-") {
			return os.Remove(name)
		}
		if len(d.Name()) != 64 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		total += info.Size()
		copies = append(copies, copyInfo{name, info.Size(), info.ModTime()})
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(copies, func(i, j int) bool { return copies[i].at.Before(copies[j].at) })
	for _, item := range copies {
		if total+reserve <= c.max {
			break
		}
		if err := os.Remove(item.name); err != nil && !os.IsNotExist(err) {
			return err
		}
		total -= item.size
	}
	return nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

type boundedCopy struct {
	ctx       context.Context
	w         io.Writer
	remaining int64
}

func (w *boundedCopy) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if int64(len(p)) > w.remaining {
		return 0, fmt.Errorf("E711 size_cap: cache transfer exceeded announced size")
	}
	n, err := w.w.Write(p)
	w.remaining -= int64(n)
	return n, err
}
