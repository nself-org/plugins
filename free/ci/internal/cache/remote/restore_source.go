package remote

// Purpose: the coordinator side of a cache restore (Epic CACHE D7, P7-CACHE-21): Lease is the
// transport.Restorer of its lease. It names the blobs of the attested plan, opens them verified
// from the lease's realm while the lease is current, and turns the transfer events into audit
// rows and cache_locality rows.
// Inputs:  the lease Binding (plan, node, epoch fence) and the blob store.
// Outputs: transport.RestoreBlob values; audit rows for refusals; a locality row per entry
// once the node confirmed every blob of it.
// Constraints: only the blobs of Plan.Restore at their planned size are ever listed; a blob
// is opened only while the lease epoch is current (a stale or cancelled lease reads nothing);
// a locality row is written after the node's confirmation, never on a send, and only under a
// current epoch; every failure is advisory and never fails the lease.

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

var _ transport.Restorer = (*Lease)(nil)

// RestoreBlobs lists the blobs of the restore plan, each opened verified on demand.
func (l *Lease) RestoreBlobs(ctx context.Context) ([]transport.RestoreBlob, error) {
	if l.b.Plan == nil || l.h == nil || l.h.Blobs == nil {
		return nil, nil
	}
	realm := cacheRealm(l.b.Plan.Namespace)
	seen := map[string]bool{}
	var out []transport.RestoreBlob
	for _, r := range l.b.Plan.Restore {
		for _, b := range r.Blobs {
			sha, size := strings.ToLower(b.SHA256), b.Size
			if seen[sha] || size < 1 {
				continue
			}
			seen[sha] = true
			out = append(out, transport.RestoreBlob{SHA256: sha, Size: size, Open: func() (io.ReadCloser, error) {
				if err := l.live(ctx); err != nil {
					return nil, err
				}
				info, err := l.h.Blobs.Stat(realm, sha)
				if err != nil {
					return nil, err
				}
				if info.Size() != size {
					return nil, fmt.Errorf("E714 digest_mismatch: the stored blob is %d B, the plan says %d B", info.Size(), size)
				}
				return l.h.Blobs.Open(realm, sha)
			}})
		}
	}
	return out, nil
}

// RestoreEvent records one step of a restore.
func (l *Lease) RestoreEvent(ctx context.Context, e transport.RestoreEvent) {
	switch e.Kind {
	case transport.RestoreRefused:
		_ = l.h.Ledger.CacheAuditOnce(ctx, "cache.remote.refused", l.b.AttemptID, "reason=restore_"+e.Reason)
	case transport.RestoreFailed:
		l.warnf("cache restore of %s: %s", e.SHA256[:min(len(e.SHA256), 12)], e.Reason)
	case transport.RestoreCopied:
		l.copied(ctx, strings.ToLower(e.SHA256))
	}
}

func (l *Lease) warnf(format string, a ...any) {
	if l.h.Warn != nil {
		l.h.Warn(fmt.Errorf(format, a...))
	}
}

// copied notes a confirmed node copy and records the locality of every entry it completes.
func (l *Lease) copied(ctx context.Context, sha string) {
	if l.b.Node == "" || l.b.Plan == nil {
		return
	}
	if err := l.live(ctx); err != nil {
		_ = l.refuse(ctx, transfer.ReasonStaleEpoch)
		return
	}
	l.mu.Lock()
	if l.restored == nil {
		l.restored = map[string]bool{}
	}
	l.restored[sha] = true
	var rows []store.CacheLocalityRow
	for _, r := range l.b.Plan.Restore {
		var total int64
		whole := len(r.Blobs) > 0
		for _, b := range r.Blobs {
			whole = whole && l.restored[strings.ToLower(b.SHA256)]
			total += b.Size
		}
		if whole {
			rows = append(rows, store.CacheLocalityRow{Node: l.b.Node, Realm: l.b.Plan.Namespace, Key: r.Key, Bytes: total})
		}
	}
	l.mu.Unlock()
	for _, row := range rows {
		if err := l.h.Ledger.CacheLocalityPut(ctx, row); err != nil {
			l.warnf("cache locality: %v", err)
		}
	}
}
