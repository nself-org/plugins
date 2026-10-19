package entries

// Purpose: the write side (Epic D4): facts and planned keys at restore time, the
// recorded result, and Commit, the only way an entry becomes visible.
// Inputs: attempt id, lease epoch, the claimed namespace, a Manifest whose blobs
// were already Put into cas realm CacheRealm(namespace). Outputs: nil, *Refused
// (E711), authz's *Denied (E684) or a plain error.
// Constraints: Commit validates the manifest, checks the blobs exist with the
// right size, enforces the free-space floor, then hands one CommitRequest to the
// backend, whose single transaction re-reads attempt state, epoch, result, the
// stored job facts and the planned save set. The caller's namespace is only
// compared; the writer fields of the manifest are replaced by the stored facts.
// A refusal writes one audit row and leaves nothing visible.

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

// RecordPlan records, while the attempt is running at epoch, the job facts the
// namespace is recomputed from and the keys the attempt may save.
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

// refuse writes the audit row for a refusal decided before the backend runs.
func (s *Store) refuse(ctx context.Context, op, target, reason string) error {
	if err := s.be.AppendAudit(ctx, "cache", "cache."+op+".refused", target, "reason="+reason); err != nil {
		return err
	}
	return &Refused{Reason: reason}
}

// Commit saves m for attempt at epoch into the namespace claimed.
func (s *Store) Commit(ctx context.Context, attempt string, epoch int64, claimed authz.Namespace, m Manifest) error {
	if !attemptID.MatchString(attempt) {
		return bad("attempt id %q", attempt)
	}
	m.Blobs = append([]Blob(nil), m.Blobs...)
	slices.SortFunc(m.Blobs, func(a, b Blob) int { return strings.Compare(a.Path, b.Path) })
	if err := m.Validate(); err != nil {
		if errors.Is(err, ErrPathOutside) {
			return s.refuse(ctx, "commit", attempt, ReasonPathOutside)
		}
		return err
	}
	s.sweep.RLock()
	defer s.sweep.RUnlock()
	realm := cas.CacheRealm(string(m.Namespace))
	for _, b := range m.Blobs {
		info, err := s.blobs.Stat(realm, b.SHA256)
		if err != nil {
			return fmt.Errorf("entries: blob %q: %w", b.Path, err)
		}
		if info.Size() != b.Size {
			return bad("blob %q has %d bytes, manifest says %d", b.Path, info.Size(), b.Size)
		}
	}
	if free, floor, ok, err := s.space(); err != nil {
		return err
	} else if ok && free-m.Bytes() < floor {
		return s.refuse(ctx, "commit", attempt, ReasonDiskFloor)
	}
	return s.be.CacheCommit(ctx, CommitRequest{Attempt: attempt, Epoch: epoch, Claimed: claimed, Manifest: m, MaxEntryBytes: s.maxBytes})
}
