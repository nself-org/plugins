package specs

// Purpose: the save half of a session (Epic CACHE D5): only for an attempt
// that passed and is finalizing, with its own namespace, a planned key and no
// restored own entry.
// Inputs:  the finalizing epoch. Outputs: entries committed through
// Engine.Save, usage rows for refusals.
// Constraints: RecordResult runs once and is write-once per run; a save that
// is refused (secret_match, size cap, disk floor, epoch, namespace) is a usage
// row and a warning, and a committed save is a `saved` usage row, never a failed attempt; the registered-value scan runs
// before the commit (Engine.Save).

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
)

// canSave reports whether the attempt may save it: its own namespace, writable
// dirs, and no own entry restored; a workspace entry also needs its staged pack.
func (it *Item) canSave() bool {
	return it.Access == authz.ModeRWOwn && it.Bound && !it.ReadOnly && !it.Hit && (!it.InWorkspace || it.staged != nil)
}

// canSaveFromWorkspace is canSave before the pack exists: AfterJob stages only
// what Finalize could commit.
func (it *Item) canSaveFromWorkspace() bool {
	return it.Access == authz.ModeRWOwn && it.Bound && !it.ReadOnly && !it.Hit
}

// Wants reports whether Finalize has anything to save.
func (s *Session) Wants() bool {
	for _, it := range s.items {
		if it.canSave() {
			return true
		}
	}
	return false
}

// Finalize saves every entry the attempt may save.
func (s *Session) Finalize(ctx context.Context, epoch int64) {
	if !s.Wants() {
		return
	}
	if err := s.in.Engine.Entries.RecordResult(ctx, s.in.Attempt, epoch, true); err != nil {
		for _, it := range s.items {
			s.use(ctx, it.Spec.ID, it.Key, resultOf(err), 0, 0)
		}
		s.warn("cache save skipped: " + reasonOf(err))
		return
	}
	for _, it := range s.items {
		if !it.canSave() {
			continue
		}
		start := time.Now()
		var n int64
		var err error
		if it.InWorkspace { // packed in AfterJob, while the workspace existed
			n, err = s.in.Engine.Commit(ctx, it.staged, epoch)
		} else {
			n, err = s.in.Engine.Save(ctx, SaveRequest{Base: s.base, Attempt: s.in.Attempt, Epoch: epoch, Namespace: s.own, Key: it.Key, Spec: it.Spec, Dirs: it.Dirs})
		}
		switch {
		case err == nil:
			s.saved = append(s.saved, it.Key)
			s.use(ctx, it.Spec.ID, it.Key, "saved", n, time.Since(start).Milliseconds())
		case errors.Is(err, ErrEmpty):
		default:
			s.use(ctx, it.Spec.ID, it.Key, resultOf(err), 0, 0)
			s.warn("cache save of spec " + it.Spec.ID + " refused: " + reasonOf(err))
		}
	}
}

// Saved reports whether this session committed an entry.
func (s *Session) Saved() bool { return len(s.saved) > 0 }

// Retract drops the entries this session committed. The hook calls it when the
// attempt did not settle `passed` (a cancel won the finalizing -> passed move
// after the commit): a cancelled attempt must leave no cache behind.
func (s *Session) Retract(ctx context.Context) {
	for _, key := range s.saved {
		list, _ := s.in.Engine.Backend.CacheEntries(ctx, key)
		for _, m := range list {
			if bytes.Equal(m.Namespace, s.own.Canonical()) && m.Writer.AttemptID == s.in.Attempt {
				_ = s.in.Engine.Backend.CacheDrop(ctx, m, "attempt_not_passed")
			}
		}
	}
	s.saved = nil
}

func resultOf(err error) string {
	var d *authz.Denied
	if errors.As(err, &d) {
		return "denied"
	}
	return "refused"
}

// reasonOf names a refusal without any value: the E711 reason, the authz
// reason, or "error".
func reasonOf(err error) string {
	var r *entries.Refused
	var d *authz.Denied
	switch {
	case errors.As(err, &r):
		return "E711 " + r.Reason
	case errors.As(err, &d):
		return "E684 " + string(d.Reason)
	}
	return "error"
}
