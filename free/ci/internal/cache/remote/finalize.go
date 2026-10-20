package remote

// Purpose: commit received cache refs during the passing finalize seam.
// Inputs: current passing attempt; bounded wait. Outputs: committed entries.
// Constraints: caller invokes only for a recorded passing job; cache is best effort.
import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Finalize waits at most wait (bounded to 30s); missing cache is best effort.
func (l *Lease) Finalize(ctx context.Context, wait time.Duration) error {
	if l.b.Plan == nil || len(l.b.Plan.Save) == 0 {
		return nil
	}
	if wait <= 0 || wait > 30*time.Second {
		wait = 30 * time.Second
	}
	grace := min(wait, 2*time.Second)
	l.mu.Lock()
	fullGrace := l.b.Unordered || len(l.refs) > 0
	l.mu.Unlock()
	if fullGrace {
		grace = wait
	}
	start := time.Now()
	timer := time.NewTimer(grace)
	defer timer.Stop()
	for {
		l.mu.Lock()
		complete := len(l.refs) >= len(l.b.Plan.Save)
		hasRefs := len(l.refs) > 0
		l.mu.Unlock()
		if complete {
			break
		}
		if hasRefs && !fullGrace {
			fullGrace = true
			timer.Reset(max(0, wait-time.Since(start)))
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
	var errs []error
	for _, r := range refs {
		if err = l.guard(ctx); err != nil {
			errs = append(errs, err)
			continue
		}
		if err = l.validateRef(ctx, r); err != nil {
			errs = append(errs, errors.Join(err, l.refuse(ctx, entries.ReasonBlobInvalid)))
			continue
		}
		m := entries.Manifest{Namespace: ns.Canonical(), Key: r.Key, Spec: r.Spec, Writer: entries.Writer{ScannedClean: true}}
		for _, b := range r.Blobs {
			m.Blobs = append(m.Blobs, entries.Blob{Path: b.Path, SHA256: b.SHA256, Size: b.Size})
		}
		if err = l.h.Entries.Commit(ctx, l.b.AttemptID, l.b.Epoch, ns, m); err != nil {
			errs = append(errs, err)
			continue
		}
		l.mu.Lock()
		l.saved = append(l.saved, m)
		l.mu.Unlock()
		if l.b.Node != "" {
			if err = l.h.Ledger.CacheLocalityPut(ctx, store.CacheLocalityRow{Node: l.b.Node, Realm: l.b.Plan.Namespace, Key: r.Key, Bytes: m.Bytes()}); err != nil {
				warning := fmt.Errorf("cache locality: %w", err)
				if l.h.Warn != nil {
					l.h.Warn(warning)
				} else {
					log.Print(warning)
				}
			}
		}
	}
	return errors.Join(errs...)
}

// validateRef applies restore's archive checks before any entry is published.
func (l *Lease) validateRef(ctx context.Context, r protocol.CacheRef) error {
	for _, b := range r.Blobs {
		f, err := l.h.Blobs.Open(cacheRealm(l.b.Plan.Namespace), b.SHA256)
		if err != nil {
			return err
		}
		err = specs.Validate(&contextReader{ctx: ctx, r: f}, authz.Limits{})
		err = errors.Join(err, f.Close())
		if err != nil {
			return fmt.Errorf("cache %s archive: %w", r.Spec, err)
		}
	}
	return nil
}

// Retract removes only entries committed by this lease when finalization loses
// the transition to passed, preserving a newer writer of the same cache key.
func (l *Lease) Retract(ctx context.Context) error {
	return l.RetractUsing(ctx, l.h.Ledger)
}

// RetractUsing permits cleanup with a fresh store after the run's store closes.
func (l *Lease) RetractUsing(ctx context.Context, ledger Ledger) error {
	l.mu.Lock()
	saved := l.saved
	l.saved = nil
	l.mu.Unlock()
	var errs []error
	for _, committed := range saved {
		rows, err := ledger.CacheEntries(ctx, committed.Key)
		errs = append(errs, err)
		for _, m := range rows {
			if bytes.Equal(m.Namespace, committed.Namespace) && m.Writer.AttemptID == l.b.AttemptID && m.Writer.Epoch == l.b.Epoch {
				errs = append(errs, ledger.CacheDrop(ctx, m, "attempt_not_passed"))
			}
		}
	}
	return errors.Join(errs...)
}
