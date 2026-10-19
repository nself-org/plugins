package entries

// Purpose: the write side (Epic D4): facts and planned keys at restore time, the
// recorded result, and Commit, the only way an entry becomes visible.
// Inputs: attempt id, capability epoch, the claimed namespace, a Manifest whose
// blobs were Put into cas realm CacheRealm(namespace) moments ago. Outputs: nil,
// *Refused (E711), authz's *Denied (E684) or a plain error.
// Constraints: Commit validates the manifest shape and hands one CommitRequest to
// the backend, whose single transaction re-reads attempt state, epoch, result,
// stored job facts and the planned save set. Only after every authorization
// check passes does the transaction run Check, which refuses path_outside, a
// blob that is missing, of the wrong size or not freshly Put (see BlobMaxAge),
// and a volume under the free-space floor. The caller's namespace is only
// compared; the writer fields of the manifest are replaced by the stored facts
// (only ScannedClean is the saver's). A refusal writes one audit row and leaves
// nothing visible.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
)

const maxPlannedKeys = 1024

// ErrBlobStale means a blob is older than BlobMaxAge: Put it again.
var ErrBlobStale = errors.New("entries: blob not freshly Put; Put it again before Commit")

// RecordPlan records, while the attempt is running at epoch, the job facts the
// namespace is recomputed from and the full set of keys the attempt may save.
// A second call replaces the set and clears any earlier result.
func (s *Store) RecordPlan(ctx context.Context, attempt string, epoch int64, f Facts, keys []string) error {
	if !attemptID.MatchString(attempt) || len(keys) > maxPlannedKeys {
		return bad("plan for attempt %q with %d keys", attempt, len(keys))
	}
	for _, k := range keys {
		if !hexKey.MatchString(k) {
			return bad("planned key is not 64 hex digits")
		}
	}
	return s.be.CacheRecordPlan(ctx, attempt, epoch, f, keys)
}

// RecordResult records the attempt's result while it is finalizing at epoch.
func (s *Store) RecordResult(ctx context.Context, attempt string, epoch int64, passed bool) error {
	return s.be.CacheRecordResult(ctx, attempt, epoch, passed)
}

// check is the in-transaction Check of one commit.
func (s *Store) check(m Manifest, shapeErr error) error {
	if errors.Is(shapeErr, ErrPathOutside) {
		return &Refused{Reason: ReasonPathOutside}
	}
	realm := cas.CacheRealm(string(m.Namespace))
	for _, b := range m.Blobs {
		info, err := s.blobs.Stat(realm, b.SHA256)
		if err != nil {
			return fmt.Errorf("entries: blob %q: %w", b.Path, err)
		}
		if info.Size() != b.Size {
			return bad("blob %q has %d bytes, manifest says %d", b.Path, info.Size(), b.Size)
		}
		if s.now().Sub(info.ModTime()) > BlobMaxAge {
			return fmt.Errorf("%w: %q", ErrBlobStale, b.Path)
		}
	}
	if free, floor, ok, err := s.space(); err != nil {
		return err
	} else if ok && free < floor {
		return &Refused{Reason: ReasonDiskFloor}
	}
	return nil
}

// Commit saves m for attempt at epoch into the namespace claimed.
func (s *Store) Commit(ctx context.Context, attempt string, epoch int64, claimed authz.Namespace, m Manifest) error {
	if !attemptID.MatchString(attempt) {
		return bad("attempt id %q", attempt)
	}
	m.Blobs = slices.Clone(m.Blobs)
	slices.SortFunc(m.Blobs, func(a, b Blob) int { return strings.Compare(a.Path, b.Path) })
	shapeErr := m.validate(false)
	if shapeErr != nil && !errors.Is(shapeErr, ErrPathOutside) {
		return shapeErr
	}
	return s.be.CacheCommit(ctx, CommitRequest{Attempt: attempt, Epoch: epoch, Claimed: claimed, Manifest: m,
		MaxEntryBytes: s.maxBytes, Check: func() error { return s.check(m, shapeErr) }})
}
