package specs

// Purpose: refusal classification and the dir packer for the cache engine (split from engine.go, D-1252).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
)

// restoreCheckError classifies why a restore of the freshly packed blob name would
// fail: size and ratio errors stay ErrTooLarge (E711 size_cap), a cancelled
// context passes through, any other path or archive error is its own reason.
func restoreCheckError(name string, err error) error {
	var pe *authz.PathError
	switch {
	case ctxDone(err):
		return err
	case errors.As(err, &pe) && strings.HasPrefix(string(pe.Reason), "size."):
		return fmt.Errorf("%w: the packed %s fails the restore checks: %v", ErrTooLarge, name, err)
	case errors.As(err, &pe):
		return &entries.Refused{Reason: entries.ReasonPathOutside, Err: fmt.Errorf("the packed %s fails the restore checks: %w", name, err)}
	}
	return &entries.Refused{Reason: entries.ReasonBlobInvalid, Err: fmt.Errorf("the packed %s fails the restore checks: %w", name, err)}
}

func ctxDone(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// refuse turns a pack or scan failure into the typed E711 refusal and writes
// the audit row for a secret match (the store writes its own for commits).
func (e *Engine) refuse(ctx context.Context, r SaveRequest, err error) error {
	var refused *entries.Refused
	switch {
	case errors.Is(err, ErrSecretMatch):
		_ = e.Backend.AppendAudit(ctx, "cache", "cache.save.refused", r.Attempt, "reason="+ReasonSecretMatch+" spec="+r.Spec.ID)
		return &entries.Refused{Reason: ReasonSecretMatch}
	case errors.Is(err, errSizeCap):
		_ = e.Backend.AppendAudit(ctx, "cache", "cache.save.refused", r.Attempt, "reason="+entries.ReasonSizeCap+" spec="+r.Spec.ID)
		return &entries.Refused{Reason: entries.ReasonSizeCap}
	case errors.As(err, &refused):
		_ = e.Backend.AppendAudit(ctx, "cache", "cache.save.refused", r.Attempt, "reason="+refused.Reason+" spec="+r.Spec.ID)
		return err
	case errors.Is(err, ErrTooLarge):
		_ = e.Backend.AppendAudit(ctx, "cache", "cache.save.refused", r.Attempt, "reason="+entries.ReasonSizeCap+" spec="+r.Spec.ID)
		return &entries.Refused{Reason: entries.ReasonSizeCap, Err: err}
	}
	return err
}

var errSizeCap = errors.New("specs: the packed cache exceeds the entry byte cap")

// capWriter counts stored bytes and fails past the cap.
type capWriter struct {
	w   io.Writer
	n   int64
	cap int64
}

func (c *capWriter) Write(p []byte) (int, error) {
	if c.n += int64(len(p)); c.n > c.cap {
		return 0, errSizeCap
	}
	return c.w.Write(p)
}

// putDir streams dir through tar and scan into the blob store. The local blob
// is a plain tar: disk is cheap and a restore is on a job's critical path, so a
// copy beats an inflate (D-1008). Compress only when a blob leaves the host.
// With inflate the scan also looks inside the gzip payloads of the files
// (Spec.InflateScan, D-1037).
func (e *Engine) putDir(ctx context.Context, base *os.Root, realm cas.Realm, rel string, budget int64, inflate bool) (string, int64, PackStats, error) {
	pr, pw := io.Pipe()
	var st PackStats
	done := make(chan struct{})
	go func() {
		defer close(done)
		var err error
		defer func() { _ = pw.CloseWithError(err) }()
		sw := newScanWriter(&capWriter{w: pw, cap: budget}, e.Scanner)
		var inf *inflater
		if inflate {
			inf = newInflater(e.Scanner)
		}
		if st, err = pack(ctx, base, rel, sw, cacheLimits(), inf); err == nil {
			err = sw.Close()
		}
	}()
	digest, size, err := e.Blobs.Put(ctx, realm, pr)
	_ = pr.CloseWithError(err)
	<-done
	return digest, size, st, err
}
