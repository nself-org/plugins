package specs

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

// SaveRequest is one entry to save. The entry's writer facts are the ones the
// backend stored at RecordPlan; only the scan result is the saver's.
type SaveRequest struct {
	Base      *os.Root
	Attempt   string
	Epoch     int64
	Namespace authz.Namespace
	Key       string
	Spec      Spec
	Dirs      Dirs
}

// Staged is an entry that was packed, scanned and put but not committed yet:
// the pack must happen while the job's directory still exists, the commit only
// while the attempt is finalizing.
type Staged struct {
	req   SaveRequest
	blobs []entries.Blob
	total int64
}

// Stage packs and scans every dir of r into the blob store without committing.
// A registered value in any dir refuses it with E711 secret_match and an audit
// row; a cache with no file in it is ErrEmpty.
func (e *Engine) Stage(ctx context.Context, r SaveRequest) (*Staged, error) {
	realm := cas.CacheRealm(string(r.Namespace.Canonical()))
	maxBytes, _ := e.Entries.Caps()
	st := &Staged{req: r}
	var files int
	for _, name := range r.Dirs.names() {
		digest, size, ps, err := e.putDir(ctx, r.Base, realm, r.Dirs[name], maxBytes-st.total, r.Spec.InflateScan)
		if err != nil {
			return nil, e.refuse(ctx, r, err)
		}
		if err := e.validate(ctx, realm, digest); err != nil { // a restore would refuse it: do not commit a loop of save, refuse, drop
			return nil, e.refuse(ctx, r, restoreCheckError(name, err))
		}
		files += ps.Files
		st.total += size
		st.blobs = append(st.blobs, entries.Blob{Path: name, SHA256: digest, Size: size})
	}
	if files == 0 {
		return nil, ErrEmpty
	}
	return st, nil
}

// Commit commits a staged entry at epoch (the attempt must be finalizing).
func (e *Engine) Commit(ctx context.Context, st *Staged, epoch int64) (int64, error) {
	r := st.req
	m := entries.Manifest{Namespace: r.Namespace.Canonical(), Key: r.Key, Spec: r.Spec.ID, Blobs: st.blobs, Writer: entries.Writer{ScannedClean: true}}
	if err := e.Entries.Commit(ctx, r.Attempt, epoch, r.Namespace, m); err != nil {
		return 0, err
	}
	return st.total, nil
}

// Save packs, scans and commits the entry. A registered value in any dir
// refuses the save with E711 secret_match and an audit row.
func (e *Engine) Save(ctx context.Context, r SaveRequest) (int64, error) {
	st, err := e.Stage(ctx, r)
	if err != nil {
		return 0, err
	}
	return e.Commit(ctx, st, r.Epoch)
}

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
// Every save also scans gzip and ZIP payloads inside the files.
func (e *Engine) putDir(ctx context.Context, base *os.Root, realm cas.Realm, rel string, budget int64, inflate bool) (string, int64, PackStats, error) {
	pr, pw := io.Pipe()
	var st PackStats
	done := make(chan struct{})
	go func() {
		defer close(done)
		var err error
		defer func() { _ = pw.CloseWithError(err) }()
		sw := newScanWriter(&capWriter{w: pw, cap: budget}, e.Scanner)
		inf := newInflater(e.Scanner)
		if st, err = pack(ctx, base, rel, sw, cacheLimits(), inf); err == nil {
			err = sw.Close()
		}
	}()
	digest, size, err := e.Blobs.Put(ctx, realm, pr)
	_ = pr.CloseWithError(err)
	<-done
	return digest, size, st, err
}
