package remote

// Purpose: verify a pinned cache copy before it can be used by a lease.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"syscall"
)

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
		return nil, errCopyChanged
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
