package remote

// Purpose: admit cache uploads and stage references until finalizing.
// Inputs: one coordinator Binding, wire blobs and refs. Outputs: fenced entries.
// Constraints: only planned keys and paths, passing result, current epoch; no SQL.
import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

const StreamCache = "cache"

type Ledger interface {
	CacheEntries(context.Context, string) ([]entries.Manifest, error)
	CacheDrop(context.Context, entries.Manifest, string) error
	AttemptStateEpoch(context.Context, string) (model.JobState, int64, error)
	LeaseGet(context.Context, string) (store.RemoteLease, error)
	CacheAuditOnce(context.Context, string, string, string) error
	CacheLocalityPut(context.Context, store.CacheLocalityRow) error
}

type Handler struct {
	Ledger  Ledger
	Entries *entries.Store
	Blobs   *cas.Store
	Limits  transfer.Limits
	Warn    func(error)
}
type Binding struct {
	AttemptID, LeaseID, Node string
	Epoch                    int64
	Facts                    entries.Facts
	Plan                     *protocol.LeaseCache
	Fence, Ready             func(context.Context) error
	// Unordered preserves the full ref grace for carriers without ordered delivery.
	Unordered bool
}

type Lease struct {
	h               *Handler
	b               Binding
	mu              sync.Mutex
	admission       sync.Mutex
	refs            map[string]protocol.CacheRef
	held            map[string]int64
	saved           []entries.Manifest
	planned, closed bool
	notify          chan struct{}
	rcv             *transfer.Receiver
}

func (h *Handler) Bind(b Binding) (*Lease, error) {
	l := &Lease{h: h, b: b, refs: map[string]protocol.CacheRef{}, notify: make(chan struct{}, 1)}
	if b.Plan == nil {
		return l, nil
	}
	if _, err := DecodePlanNamespace(b.Plan.Namespace); err != nil {
		return nil, fmt.Errorf("cache plan namespace: %w", err)
	}
	lim := h.Limits
	if lim.MaxFile <= 0 {
		lim.MaxFile, _ = h.Entries.Caps()
	}
	capBytes, _ := h.Entries.Caps()
	l.held = map[string]int64{}
	lim.Budget = func(_ context.Context, sha string) (int64, error) {
		l.mu.Lock()
		defer l.mu.Unlock()
		left := capBytes
		for d, n := range l.held {
			if d != sha {
				left -= n
			}
		}
		return max(left, 0), nil
	}
	l.rcv = &transfer.Receiver{Store: h.Blobs, Realm: cacheRealm(b.Plan.Namespace), Lease: fmt.Sprintf("cache/%s/%d", b.LeaseID, b.Epoch), Limits: lim,
		Live: l.live, Refused: func(ctx context.Context, _ string, e *transfer.Error) {
			_ = h.Ledger.CacheAuditOnce(ctx, "cache.remote.refused", b.AttemptID, "reason="+e.Reason)
		}}
	return l, nil
}

func (l *Lease) refuse(ctx context.Context, reason string) error {
	auditErr := l.h.Ledger.CacheAuditOnce(ctx, "cache.remote.refused", l.b.AttemptID, "reason="+reason)
	if reason == transfer.ReasonStaleEpoch {
		return errors.Join(transfer.Failf(reason, "the cache lease is no longer current"), auditErr)
	}
	return errors.Join(&entries.Refused{Reason: reason}, auditErr)
}

func (l *Lease) live(ctx context.Context) error {
	// The carrier fence intentionally closes at the result transition. Cache
	// refs may arrive during finalizing, so compare the durable lease directly;
	// entries.Commit repeats that epoch check in its transaction.
	durable, err := l.h.Ledger.LeaseGet(ctx, l.b.LeaseID)
	if err != nil {
		var coded *store.Error
		if errors.As(err, &coded) && coded.Code == "E662" {
			return errors.Join(transfer.ErrStale, err)
		}
		return err
	}
	if durable.Epoch != l.b.Epoch || durable.AttemptID != l.b.AttemptID {
		return transfer.ErrStale
	}
	if durable.State == "finalizing" {
		return nil
	}
	if l.b.Fence == nil {
		return transfer.ErrStale
	}
	if err := l.b.Fence(ctx); err != nil {
		return err
	}
	state, _, err := l.h.Ledger.AttemptStateEpoch(ctx, l.b.AttemptID)
	if err != nil {
		return err
	}
	if state != "leased" && state != "running" && state != "finalizing" {
		return transfer.ErrStale
	}
	return nil
}

func (l *Lease) guard(ctx context.Context) error {
	if l.b.Plan == nil {
		return l.refuse(ctx, entries.ReasonNotPlanned)
	}
	if err := l.live(ctx); err != nil {
		if errors.Is(err, transfer.ErrStale) {
			return l.refuse(ctx, transfer.ReasonStaleEpoch)
		}
		return err
	}
	if l.b.Plan.Access != "rw" || l.b.Facts.Trust == "untrusted" {
		return l.refuse(ctx, entries.ReasonNotPlanned)
	}
	return nil
}

// Declare records the complete save set before the result transition.
func (l *Lease) Declare(ctx context.Context) error {
	if l.b.Plan == nil {
		return l.refuse(ctx, entries.ReasonNotPlanned)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.planned {
		return nil
	}
	if l.b.Ready != nil {
		if err := l.b.Ready(ctx); err != nil {
			return err
		}
	}
	if err := l.live(ctx); err != nil {
		return err
	}
	var keys []string
	for _, s := range l.b.Plan.Save {
		keys = append(keys, s.Key)
	}
	if err := l.h.Entries.RecordPlan(ctx, l.b.AttemptID, l.b.Epoch, l.b.Facts, keys); err != nil {
		return err
	}
	l.planned = true
	return nil
}

// Blob adapts the existing artifact receiver without changing the wire stream.
func (l *Lease) Blob(ctx context.Context, h transport.BlobHeader) (transport.BlobWriter, error) {
	l.admission.Lock()
	defer l.admission.Unlock()
	if h.Stream != StreamCache {
		return nil, transfer.Failf(transfer.ReasonProtocol, "not a cache blob")
	}
	if err := l.guard(ctx); err != nil {
		return nil, err
	}
	if len(l.b.Plan.Save) == 0 {
		return nil, l.refuse(ctx, entries.ReasonNotPlanned)
	}
	if err := l.Declare(ctx); err != nil {
		return nil, err
	}
	h.Stream = transfer.StreamArtifact
	w, err := l.rcv.Open(ctx, h)
	if err == nil {
		l.mu.Lock()
		l.held[h.SHA256] = h.Size
		l.mu.Unlock()
	}
	return w, err
}

func (l *Lease) CacheRef(ctx context.Context, r protocol.CacheRef) error {
	if err := l.guard(ctx); err != nil {
		return err
	}
	if err := l.Declare(ctx); err != nil {
		return err
	}
	var save *protocol.CacheSave
	for i := range l.b.Plan.Save {
		s := &l.b.Plan.Save[i]
		if s.Key == r.Key && s.Spec == r.Spec {
			save = s
			break
		}
	}
	if r.LeaseID != l.b.LeaseID || save == nil {
		return l.refuse(ctx, entries.ReasonNotPlanned)
	}
	if len(r.Blobs) != len(save.Dirs) {
		return l.refuse(ctx, entries.ReasonNotPlanned)
	}
	seen := map[string]bool{}
	for _, b := range r.Blobs {
		if b.Size < 1 {
			return l.refuse(ctx, entries.ReasonBlobInvalid)
		}
		found := false
		for _, d := range save.Dirs {
			if d == r.Spec+"/"+b.Path {
				found = true
			}
		}
		if !found || seen[b.Path] {
			return l.refuse(ctx, entries.ReasonNotPlanned)
		}
		seen[b.Path] = true
		fi, err := l.h.Blobs.Stat(cacheRealm(l.b.Plan.Namespace), b.SHA256)
		if err != nil || fi.Size() != b.Size {
			return l.refuse(ctx, entries.ReasonBlobInvalid)
		}
		if err = l.h.Blobs.Refresh(cacheRealm(l.b.Plan.Namespace), b.SHA256); err != nil {
			return l.refuse(ctx, entries.ReasonBlobInvalid)
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return l.refuse(ctx, entries.ReasonAttemptState)
	}
	r.Blobs = append([]protocol.CacheBlob(nil), r.Blobs...)
	l.refs[r.Key] = r
	select {
	case l.notify <- struct{}{}:
	default:
	}
	return nil
}

func DecodePlanNamespace(wire string) (authz.Namespace, error) {
	b, err := base64.RawURLEncoding.DecodeString(wire)
	if err != nil {
		return authz.Namespace{}, err
	}
	return authz.DecodeNamespace(b)
}
func cacheRealm(wire string) cas.Realm {
	ns, _ := DecodePlanNamespace(wire)
	return cas.CacheRealm(string(ns.Canonical()))
}
