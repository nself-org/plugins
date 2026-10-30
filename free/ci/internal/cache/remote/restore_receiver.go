package remote

// Purpose: receive one lease's cache restore stream into the read-through copies.
// Inputs: digest announcements and chunk{stream:cache} frames with their epoch.
// Outputs: verified copies in the lease's realm; audit rows for every refusal.
// Constraints: only digests in the lease's attested restore plan; current epoch
// only; nothing after cancel; every copy is re-hashed and corrupt ones discarded.

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// Restore refusal audit action and the reason a cancelled lease reports.
const (
	AuditRestoreRefused  = "cache.restore.refused"
	ReasonLeaseCancelled = "lease_cancelled"
)

// RestoreAuditor records one refusal row; the coordinator Ledger satisfies it.
type RestoreAuditor interface {
	CacheAuditOnce(ctx context.Context, action, target, detail string) error
}

// RestoreReceiver accepts the restore stream of a single lease. It is safe for
// concurrent use; frames of one digest must arrive in offset order.
type RestoreReceiver struct {
	copies  *ReadThrough
	audit   RestoreAuditor
	leaseID string
	epoch   uint64
	realm   string
	planned map[string]int64

	mu        sync.Mutex
	cancelled bool
	cur       *restoreSpool
	done      map[string]bool
}

type restoreSpool struct {
	digest string
	size   int64
	file   *os.File
	n      int64
}

// NewRestoreReceiver binds a receiver to the lease's restore plan and realm.
func NewRestoreReceiver(copies *ReadThrough, lease protocol.Lease, audit RestoreAuditor) (*RestoreReceiver, error) {
	if copies == nil || lease.LeaseID == "" || lease.Cache == nil {
		return nil, &entries.Refused{Reason: entries.ReasonNotPlanned}
	}
	if _, err := DecodePlanNamespace(lease.Cache.Namespace); err != nil {
		return nil, &entries.Refused{Reason: entries.ReasonNamespace, Err: err}
	}
	planned := map[string]int64{}
	for _, r := range lease.Cache.Restore {
		for _, b := range r.Blobs {
			if prev, ok := planned[b.SHA256]; (ok && prev != b.Size) || b.Size < 0 {
				return nil, &entries.Refused{Reason: entries.ReasonBlobInvalid}
			}
			planned[b.SHA256] = b.Size
		}
	}
	return &RestoreReceiver{copies: copies, audit: audit, leaseID: lease.LeaseID, epoch: lease.Epoch,
		realm: lease.Cache.Namespace, planned: planned, done: map[string]bool{}}, nil
}

// Realm is the read-through realm every accepted copy is written under.
func (r *RestoreReceiver) Realm() string { return r.realm }

// Received reports whether digest was verified and published by this receiver.
func (r *RestoreReceiver) Received(digest string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.done[digest]
}

// Cancel fences the lease: any partial copy is discarded and later frames refused.
func (r *RestoreReceiver) Cancel() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancelled = true
	r.discard()
}

// Close discards a partial copy without fencing the lease.
func (r *RestoreReceiver) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.discard()
	return nil
}

// Digest announces the next blob; it replaces (and discards) any partial one.
func (r *RestoreReceiver) Digest(ctx context.Context, epoch uint64, d protocol.Digest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.admit(ctx, epoch, d.Stream, d.SHA256); err != nil {
		return err
	}
	if size := r.planned[d.SHA256]; size != d.Size {
		return r.refuse(ctx, entries.ReasonBlobInvalid, d.SHA256)
	}
	if d.Size > r.copies.max {
		return r.refuse(ctx, entries.ReasonSizeCap, d.SHA256)
	}
	r.discard()
	f, err := os.CreateTemp(r.copies.dir, ".restore-")
	if err != nil {
		return err
	}
	r.cur = &restoreSpool{digest: d.SHA256, size: d.Size, file: f}
	return nil
}

// Chunk appends one frame to the announced blob. On the final frame the copy is
// re-hashed and published, or discarded when it does not match.
func (r *RestoreReceiver) Chunk(ctx context.Context, epoch uint64, c protocol.Chunk) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.admit(ctx, epoch, c.Stream, c.Digest); err != nil {
		return err
	}
	s := r.cur
	if s == nil || s.digest != c.Digest || c.Offset != s.n {
		r.discard()
		return r.refuse(ctx, entries.ReasonBlobInvalid, c.Digest)
	}
	data, err := base64.StdEncoding.DecodeString(c.Data)
	if err != nil {
		r.discard()
		return r.refuse(ctx, entries.ReasonBlobInvalid, c.Digest)
	}
	if int64(len(data)) > s.size-s.n {
		r.discard()
		return r.refuse(ctx, entries.ReasonSizeCap, c.Digest)
	}
	if _, err := s.file.Write(data); err != nil {
		r.discard()
		return err
	}
	s.n += int64(len(data))
	if !c.Final {
		return nil
	}
	defer r.discard()
	if s.n != s.size {
		return r.refuse(ctx, entries.ReasonBlobInvalid, c.Digest)
	}
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	f, err := r.copies.Open(ctx, r.realm, s.digest, s.size, func(ctx context.Context, w io.Writer) error {
		_, err := io.Copy(w, &contextReader{ctx: ctx, r: s.file})
		return err
	})
	if errors.Is(err, errCopyMismatch) {
		return r.refuse(ctx, entries.ReasonBlobInvalid, c.Digest)
	}
	if err != nil {
		return err
	}
	_ = f.Close()
	r.done[s.digest] = true
	return nil
}

// admit applies the cancel guard, epoch fence and plan check, in that order.
func (r *RestoreReceiver) admit(ctx context.Context, epoch uint64, stream, digest string) error {
	switch {
	case r.cancelled:
		return r.refuse(ctx, ReasonLeaseCancelled, digest)
	case epoch != r.epoch:
		r.discard()
		return r.refuse(ctx, entries.ReasonEpoch, digest)
	case stream != StreamCache:
		return r.refuse(ctx, entries.ReasonNotPlanned, digest)
	}
	if _, ok := r.planned[digest]; !ok {
		return r.refuse(ctx, entries.ReasonNotPlanned, digest)
	}
	return nil
}

// refuse writes the audit row and returns the E711 refusal, joined with any
// audit failure so a lost row is never silent.
func (r *RestoreReceiver) refuse(ctx context.Context, reason, digest string) error {
	refused := &entries.Refused{Reason: reason}
	if r.audit == nil {
		return refused
	}
	target := r.leaseID + "/" + digest
	if err := r.audit.CacheAuditOnce(ctx, AuditRestoreRefused, target, reason); err != nil {
		return errors.Join(refused, fmt.Errorf("restore audit: %w", err))
	}
	return refused
}

func (r *RestoreReceiver) discard() {
	if r.cur == nil {
		return
	}
	name := r.cur.file.Name()
	_ = r.cur.file.Close()
	_ = os.Remove(filepath.Clean(name))
	r.cur = nil
}
