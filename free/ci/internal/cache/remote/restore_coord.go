package remote

// Purpose: the coordinator half of a cache restore (P7-CACHE-21, Epic CACHE D7):
// push the blobs the lease plan restores to the node over the reverse blob path
// (digest, then chunk frames; the node answers offset), and report the copies
// the node confirmed to cache_locality.
// Inputs:  the lease's Binding (plan, epoch fence, node) and a transport.RestoreLink.
// Outputs: digest and chunk frames; locality rows; audit rows for refusals.
// Constraints: only a blob named by the lease plan, in the plan realm, and only
// while the lease epoch is current and the attempt is leased or running; a blob
// is done when the node confirms it holds every byte (a held copy answers at the
// announce, a fetched one after its re-hash). A node that reports a failed
// re-hash after the final chunk gets the blob again, as many times as the
// transfer allows. Locality is advisory and never authorises a read.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// DefaultRestoreWait bounds each wait for the node's answer.
const DefaultRestoreWait = 30 * time.Second

// restorePlanned returns the plan entry of a restore blob.
func (l *Lease) restorePlanned(sha string) (protocol.CacheBlob, bool) {
	if l.b.Plan == nil {
		return protocol.CacheBlob{}, false
	}
	for _, r := range l.b.Plan.Restore {
		for _, b := range r.Blobs {
			if b.SHA256 == sha {
				return b, true
			}
		}
	}
	return protocol.CacheBlob{}, false
}

// OpenRestore opens one planned restore blob for sending. Every refusal leaves
// an audit row: a digest outside the plan, a stale epoch, a lease no longer
// running, or a cancelled request.
func (l *Lease) OpenRestore(ctx context.Context, sha string) (io.ReadCloser, protocol.CacheBlob, error) {
	b, ok := l.restorePlanned(sha)
	if !ok {
		return nil, b, l.refuse(ctx, entries.ReasonNotPlanned)
	}
	if ctx.Err() != nil {
		return nil, b, l.refuse(context.WithoutCancel(ctx), "cancelled")
	}
	if err := l.live(ctx); err != nil {
		if errors.Is(err, transfer.ErrStale) {
			return nil, b, l.refuse(ctx, transfer.ReasonStaleEpoch)
		}
		return nil, b, err
	}
	state, _, err := l.h.Ledger.AttemptStateEpoch(ctx, l.b.AttemptID)
	if err != nil {
		return nil, b, err
	}
	if state != "leased" && state != "running" {
		return nil, b, l.refuse(ctx, entries.ReasonAttemptState)
	}
	realm := cacheRealm(l.b.Plan.Namespace)
	info, err := l.h.Blobs.Stat(realm, sha)
	if err != nil {
		return nil, b, err
	}
	if info.Size() != b.Size {
		return nil, b, transfer.Failf(transfer.ReasonDigest, "the coordinator holds blob %s at %d B, the plan says %d B", sha[:12], info.Size(), b.Size)
	}
	rc, err := l.h.Blobs.Open(realm, sha) // re-hashes into a snapshot before the first byte is served
	return rc, b, err
}

// Restore sends the plan's restore blobs to the node, one entry at a time, and
// records the entries the node confirmed. It returns the first error; the node
// degrades the entries that did not arrive to a cold miss.
func (l *Lease) Restore(ctx context.Context, link transport.RestoreLink) error {
	if l.b.Plan == nil || len(l.b.Plan.Restore) == 0 {
		return nil
	}
	wait := l.b.RestoreWait
	if wait <= 0 {
		wait = DefaultRestoreWait
	}
	tee := newOffsetTee(ctx, link.Offsets())
	up := &finalSnap{link: link, tee: tee}
	snd := &transfer.Sender{Up: up, Offsets: tee.out, WaitOffset: wait}
	done := map[string]bool{}
	for _, r := range l.b.Plan.Restore {
		var total int64
		for _, b := range r.Blobs {
			total += b.Size
			if done[b.SHA256] {
				continue
			}
			if err := l.sendRestore(ctx, snd, up, tee, b, wait); err != nil {
				return err
			}
			done[b.SHA256] = true
		}
		if l.b.Node != "" {
			// Realm is the wire namespace, as finalize.go writes it at commit (not cas.CacheRealm).
			row := store.CacheLocalityRow{Node: l.b.Node, Realm: l.b.Plan.Namespace, Key: r.Key, Bytes: total}
			if err := l.h.Ledger.CacheLocalityPut(ctx, row); err != nil && l.h.Warn != nil {
				l.h.Warn(fmt.Errorf("cache locality: %w", err))
			}
		}
	}
	return nil
}

func (l *Lease) sendRestore(ctx context.Context, snd *transfer.Sender, up *finalSnap, tee *offsetTee, b protocol.CacheBlob, wait time.Duration) error {
	blob := transfer.Blob{Stream: StreamCache, SHA256: b.SHA256, Size: b.Size, Open: func() (io.ReadCloser, error) {
		rc, _, err := l.OpenRestore(ctx, b.SHA256)
		return rc, err
	}}
	tries := transfer.MaxResumes + 1
	for i := 0; i < tries; i++ {
		up.reset()
		if err := snd.Send(ctx, blob); err != nil {
			return err
		}
		since, streamed := up.final()
		if !streamed { // the node held a verified copy: its announce answer was the whole offset
			return nil
		}
		got, err := tee.after(ctx, b.SHA256, since, wait)
		if err != nil {
			return err
		}
		if got == b.Size {
			return nil
		}
		// The node re-hashed the bytes, found them wrong, discarded them and reset to 0: send again.
	}
	return transfer.Failf(transfer.ReasonDigest, "blob %s failed verification on the node %d times", b.SHA256[:12], tries)
}
