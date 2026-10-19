package specs

// Purpose: restore and save one spec entry (Epic CACHE D5): one plain tar
// blob (a gzip blob still restores) per spec dir, put into the cache realm of the entry's
// namespace, committed through entries.Commit.
// Inputs:  entries.Store, cas.Store, an authz.ValueScanner, a Backend for the
// audit rows this package writes itself.
// Outputs: Engine.Restore, Engine.Save, ErrEmpty, ReasonSecretMatch.
// Constraints: a save streams tar -> scan -> cas.Put, so a registered
// value aborts the stream before any blob is visible, and Commit is called
// only after every blob was Put; nothing is committed for a cache with no
// files in it (a hit on an empty entry would keep the next run from filling
// it); restore refuses (and drops) an entry whose blob set is not exactly the
// spec's dirs or whose archive the path sanitiser rejects.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"

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
	m, ok, err := e.Entries.LookupForRestore(ctx, ns, key, access)
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
		err = e.unpackBlobs(ctx, base, realm, m.Blobs, dirs, opt)
	} else {
		err = errors.New("the entry's blobs are not this spec's directories")
	}
	if err == nil {
		return true, m.Bytes(), nil
	}
	for _, d := range dirs {
		_ = removeIn(base, d)
	}
	if ctx.Err() != nil { // a cancelled restore says nothing about the entry: keep it
		return false, 0, ctx.Err()
	}
	if access == authz.ModeRWOwn { // a lower-trust reader never deletes a higher-trust entry, it only audits
		_ = e.Backend.CacheDrop(ctx, m, "unrestorable")
	}
	_ = e.Backend.AppendAudit(ctx, "cache", "cache.restore.refused", key, "reason="+refusalKey(err))
	return false, 0, fmt.Errorf("cache restore refused: %w", err)
}

// writerPassed reports whether the attempt that committed m settled passed. An
// attempt still finalizing is not served yet; one that ended any other way, or
// that the store does not know, has its entry dropped: a missing attempt is no
// evidence of a pass (a cancel can win finalizing -> passed after the commit),
// and a history prune retires the entries of the attempts it removes.
func (e *Engine) writerPassed(ctx context.Context, m entries.Manifest) bool {
	state, _, err := e.Backend.AttemptStateEpoch(ctx, m.Writer.AttemptID)
	switch {
	case err != nil:
		// any failure but a missing row serves nothing and drops nothing
		// (database/sql is not importable here, so the missing row is
		// recognised by its message)
		if !strings.Contains(err.Error(), "no rows in result set") {
			return false
		}
	case state == "passed":
		return true
	case state == "queued" || state == "leased" || state == "running" || state == "finalizing":
		return false
	}
	_ = e.Backend.CacheDrop(ctx, m, "attempt_not_passed")
	return false
}

// validate runs the restore checks over a freshly Put blob without writing it anywhere.
func (e *Engine) validate(ctx context.Context, realm cas.Realm, digest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return e.Blobs.ReadVerified(realm, digest, func(r io.Reader) error {
		return Validate(&ctxReader{ctx, r}, cacheLimits())
	})
}

// unpackBlobs restores every blob of an entry at the same time (at most four):
// each goes to its own directory, so a build cache and a module cache unpack
// side by side (D-1008). The first blob's error is the one reported.
func (e *Engine) unpackBlobs(ctx context.Context, base *os.Root, realm cas.Realm, blobs []entries.Blob, dirs Dirs, opt UnpackOptions) error {
	errs := make([]error, len(blobs))
	var wg sync.WaitGroup
	slots := make(chan struct{}, 4)
	for i, b := range blobs {
		slots <- struct{}{}
		wg.Go(func() {
			defer func() { <-slots }()
			errs[i] = e.unpackBlob(ctx, base, realm, b.SHA256, dirs[b.Path], opt)
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// unpackBlob restores one blob. The store verifies its digest while Unpack reads
// it (one pass over the file, no snapshot copy: D-1008); the tree built from a
// blob that fails the check is discarded by the caller (Restore empties the dirs).
func (e *Engine) unpackBlob(ctx context.Context, base *os.Root, realm cas.Realm, digest, rel string, opt UnpackOptions) error {
	return e.Blobs.ReadVerified(realm, digest, func(r io.Reader) error {
		_, err := Unpack(&ctxReader{ctx, r}, base, rel, cacheLimits(), opt)
		return err
	})
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
