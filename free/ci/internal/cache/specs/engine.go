package specs

// Purpose: restore and save one spec entry (Epic CACHE D5): one tar + gzip
// level 1 blob per spec dir, put into the cache realm of the entry's
// namespace, committed through entries.Commit.
// Inputs:  entries.Store, cas.Store, an authz.ValueScanner, a Backend for the
// audit rows this package writes itself.
// Outputs: Engine.Restore, Engine.Save, ErrEmpty, ReasonSecretMatch.
// Constraints: a save streams tar -> scan -> gzip -> cas.Put, so a registered
// value aborts the stream before any blob is visible, and Commit is called
// only after every blob was Put; nothing is committed for a cache with no
// files in it (a hit on an empty entry would keep the next run from filling
// it); restore refuses (and drops) an entry whose blob set is not exactly the
// spec's dirs or whose archive the path sanitiser rejects.

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// ReasonSecretMatch is the E711 reason of a save refused by the value scan.
const ReasonSecretMatch = "secret_match"

// ErrEmpty means no spec dir holds a file; nothing is saved.
var ErrEmpty = errors.New("specs: the cache directories are empty")

// Backend is the persistence the engine needs beyond entries.Store.
type Backend interface {
	AttemptStateEpoch(ctx context.Context, id string) (model.JobState, int64, error)
	CacheEntries(ctx context.Context, key string) ([]entries.Manifest, error)
	CacheDrop(ctx context.Context, m entries.Manifest, reason string) error
	AppendAudit(ctx context.Context, actor, action, target, detail string) error
}

// Engine saves and restores entries. All fields are required.
type Engine struct {
	Entries *entries.Store
	Blobs   *cas.Store
	Scanner authz.ValueScanner
	Backend Backend
}

// Dirs maps a spec dir name (the blob path) to its directory relative to the
// attempt's pinned cache root.
type Dirs map[string]string

func (d Dirs) names() []string {
	out := make([]string, 0, len(d))
	for n := range d {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Restore looks key up in ns under access and unpacks a hit into dirs. A miss
// is (false, nil). A refused entry is dropped, the dirs are emptied and the
// error carries the sanitiser's E683 or the reason.
func (e *Engine) Restore(ctx context.Context, base *os.Root, ns authz.Namespace, key string, access authz.Mode, spec Spec, dirs Dirs, opt UnpackOptions) (hit bool, bytes int64, err error) {
	m, ok, err := e.Entries.Lookup(ctx, ns, key, access)
	if err != nil || !ok {
		return false, 0, err
	}
	if !e.writerPassed(ctx, m) { // a cancel won after the commit and nobody retracted it (or a crash): never serve it
		return false, 0, nil
	}
	bad := m.Spec != spec.ID || len(m.Blobs) != len(dirs)
	for _, b := range m.Blobs {
		if _, known := dirs[b.Path]; !known {
			bad = true
		}
	}
	realm := cas.CacheRealm(string(m.Namespace))
	if !bad {
		for _, b := range m.Blobs {
			if err = e.unpackBlob(base, realm, b.SHA256, dirs[b.Path], opt); err != nil {
				break
			}
		}
	} else {
		err = errors.New("the entry's blobs are not this spec's directories")
	}
	if err == nil {
		return true, m.Bytes(), nil
	}
	for _, d := range dirs {
		_ = removeIn(base, d)
	}
	if access == authz.ModeRWOwn { // a lower-trust reader never deletes a higher-trust entry, it only audits
		_ = e.Backend.CacheDrop(ctx, m, "unrestorable")
	}
	_ = e.Backend.AppendAudit(ctx, "cache", "cache.restore.refused", key, "reason="+refusalKey(err))
	return false, 0, fmt.Errorf("cache restore refused: %w", err)
}

// writerPassed reports whether the attempt that committed m settled passed. An
// attempt still finalizing is not served yet; one that ended any other way has
// its entry dropped. An attempt the store no longer knows (history pruned) is
// trusted: the commit itself required a recorded pass.
func (e *Engine) writerPassed(ctx context.Context, m entries.Manifest) bool {
	state, _, err := e.Backend.AttemptStateEpoch(ctx, m.Writer.AttemptID)
	switch {
	case err != nil || state == "passed":
		return true
	case state == "queued" || state == "leased" || state == "running" || state == "finalizing":
		return false
	}
	_ = e.Backend.CacheDrop(ctx, m, "attempt_not_passed")
	return false
}

// validate runs the restore checks over a freshly Put blob without writing it anywhere.
func (e *Engine) validate(realm cas.Realm, digest string) error {
	rc, err := e.Blobs.Open(realm, digest)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	return Validate(rc, cacheLimits())
}

func (e *Engine) unpackBlob(base *os.Root, realm cas.Realm, digest, rel string, opt UnpackOptions) error {
	rc, err := e.Blobs.Open(realm, digest)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	_, err = Unpack(rc, base, rel, cacheLimits(), opt)
	return err
}

func refusalKey(err error) string {
	var pe *authz.PathError
	if errors.As(err, &pe) {
		return string(pe.Reason)
	}
	return "unreadable"
}

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

// Save packs, scans and commits the entry. A registered value in any dir
// refuses the save with E711 secret_match and an audit row.
func (e *Engine) Save(ctx context.Context, r SaveRequest) (int64, error) {
	realm := cas.CacheRealm(string(r.Namespace.Canonical()))
	maxBytes, _ := e.Entries.Caps()
	var blobs []entries.Blob
	var files int
	var total int64
	for _, name := range r.Dirs.names() {
		digest, size, st, err := e.putDir(ctx, r.Base, realm, r.Dirs[name], maxBytes-total)
		if err != nil {
			return 0, e.refuse(ctx, r, err)
		}
		if err := e.validate(realm, digest); err != nil { // a restore would refuse it: do not commit a loop of save, refuse, drop
			return 0, e.refuse(ctx, r, fmt.Errorf("%w: the packed %s fails the restore checks: %v", ErrTooLarge, name, err))
		}
		files += st.Files
		total += size
		blobs = append(blobs, entries.Blob{Path: name, SHA256: digest, Size: size})
	}
	if files == 0 {
		return 0, ErrEmpty
	}
	m := entries.Manifest{Namespace: r.Namespace.Canonical(), Key: r.Key, Spec: r.Spec.ID, Blobs: blobs, Writer: entries.Writer{ScannedClean: true}}
	if err := e.Entries.Commit(ctx, r.Attempt, r.Epoch, r.Namespace, m); err != nil {
		return 0, err
	}
	return total, nil
}

// refuse turns a pack or scan failure into the typed E711 refusal and writes
// the audit row for a secret match (the store writes its own for commits).
func (e *Engine) refuse(ctx context.Context, r SaveRequest, err error) error {
	switch {
	case errors.Is(err, ErrSecretMatch):
		_ = e.Backend.AppendAudit(ctx, "cache", "cache.save.refused", r.Attempt, "reason="+ReasonSecretMatch+" spec="+r.Spec.ID)
		return &entries.Refused{Reason: ReasonSecretMatch}
	case errors.Is(err, errSizeCap):
		_ = e.Backend.AppendAudit(ctx, "cache", "cache.save.refused", r.Attempt, "reason="+entries.ReasonSizeCap+" spec="+r.Spec.ID)
		return &entries.Refused{Reason: entries.ReasonSizeCap}
	case errors.Is(err, ErrTooLarge):
		_ = e.Backend.AppendAudit(ctx, "cache", "cache.save.refused", r.Attempt, "reason="+entries.ReasonSizeCap+" spec="+r.Spec.ID)
		return &entries.Refused{Reason: entries.ReasonSizeCap, Err: err}
	}
	return err
}

var errSizeCap = errors.New("specs: the packed cache exceeds the entry byte cap")

// capWriter counts compressed bytes and fails past the cap.
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

// putDir streams dir through tar, scan and gzip into the blob store.
func (e *Engine) putDir(ctx context.Context, base *os.Root, realm cas.Realm, rel string, budget int64) (string, int64, PackStats, error) {
	pr, pw := io.Pipe()
	var st PackStats
	done := make(chan struct{})
	go func() {
		defer close(done)
		var err error
		defer func() { _ = pw.CloseWithError(err) }()
		gz, _ := gzip.NewWriterLevel(&capWriter{w: pw, cap: budget}, gzip.BestSpeed)
		sw := newScanWriter(gz, e.Scanner)
		if st, err = Pack(ctx, base, rel, sw, cacheLimits()); err == nil {
			err = sw.Close()
		}
		if err == nil {
			err = gz.Close()
		}
	}()
	digest, size, err := e.Blobs.Put(ctx, realm, pr)
	_ = pr.CloseWithError(err)
	<-done
	return digest, size, st, err
}
