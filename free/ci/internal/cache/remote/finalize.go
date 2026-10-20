package remote

// Purpose: commit received cache refs during the passing finalize seam.
// Inputs: current passing attempt; bounded wait. Outputs: committed entries.
// Constraints: caller invokes only for a recorded passing job; cache is best effort.
import (
	"bytes"
	"context"
	"errors"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"time"
)

// Finalize waits at most wait (bounded to 30s); missing cache is best effort.
func (l *Lease) Finalize(ctx context.Context, wait time.Duration) error {
	if l.b.Plan == nil || len(l.b.Plan.Save) == 0 {
		return nil
	}
	if wait <= 0 || wait > 30*time.Second {
		wait = 30 * time.Second
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		l.mu.Lock()
		complete := len(l.refs) >= len(l.b.Plan.Save)
		l.mu.Unlock()
		if complete {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			goto commit
		case <-l.notify:
		}
	}
commit:
	l.mu.Lock()
	l.closed = true
	refs := make([]protocol.CacheRef, 0, len(l.refs))
	for _, r := range l.refs {
		refs = append(refs, r)
	}
	l.mu.Unlock()
	if err := l.guard(ctx); err != nil {
		return err
	}
	if err := l.h.Entries.RecordResult(ctx, l.b.AttemptID, l.b.Epoch, true); err != nil {
		return err
	}
	ns, err := DecodePlanNamespace(l.b.Plan.Namespace)
	if err != nil {
		return err
	}
	for _, r := range refs {
		if err = l.guard(ctx); err != nil {
			return err
		}
		m := entries.Manifest{Namespace: ns.Canonical(), Key: r.Key, Spec: r.Spec, Writer: entries.Writer{ScannedClean: true}}
		for _, b := range r.Blobs {
			m.Blobs = append(m.Blobs, entries.Blob{Path: b.Path, SHA256: b.SHA256, Size: b.Size})
		}
		if err = l.h.Entries.Commit(ctx, l.b.AttemptID, l.b.Epoch, ns, m); err != nil {
			return err
		}
		l.mu.Lock()
		l.saved = append(l.saved, m)
		l.mu.Unlock()
		if l.b.Node != "" {
			if err = l.h.Ledger.CacheLocalityPut(ctx, store.CacheLocalityRow{Node: l.b.Node, Realm: l.b.Plan.Namespace, Key: r.Key, Bytes: m.Bytes()}); err != nil {
				return err
			}
		}
	}
	return nil
}

// Retract removes only entries committed by this lease when finalization loses
// the transition to passed, preserving a newer writer of the same cache key.
func (l *Lease) Retract(ctx context.Context) error {
	l.mu.Lock()
	saved := l.saved
	l.saved = nil
	l.mu.Unlock()
	var errs []error
	for _, committed := range saved {
		rows, err := l.h.Ledger.CacheEntries(ctx, committed.Key)
		errs = append(errs, err)
		for _, m := range rows {
			if bytes.Equal(m.Namespace, committed.Namespace) && m.Writer.AttemptID == l.b.AttemptID && m.Writer.Epoch == l.b.Epoch {
				errs = append(errs, l.h.Ledger.CacheDrop(ctx, m, "attempt_not_passed"))
			}
		}
	}
	return errors.Join(errs...)
}
