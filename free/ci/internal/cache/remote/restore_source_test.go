package remote

// Purpose: the coordinator side of a restore: the plan is the only list of blobs, a blob opens only
// while the lease epoch is current, refusals leave an audit row, and a locality row appears only
// after the node confirmed every blob of an entry.

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

var restoreEntryKey = strings.Repeat("c", 64)

// restoreFixture binds the coordinator fixture to a restore plan of two blobs in one entry.
func restoreFixture(t *testing.T) (*Lease, *store.Store, [2]protocol.CacheBlob, [2][]byte) {
	t.Helper()
	l, s, _ := coordinatorFixture(t)
	l.b.Node = "node"
	realm := cacheRealm(l.b.Plan.Namespace)
	var blobs [2]protocol.CacheBlob
	datas := [2][]byte{bytes.Repeat([]byte("one "), 100), bytes.Repeat([]byte("two "), 200)}
	for i, d := range datas {
		sha, size, err := l.h.Blobs.Put(context.Background(), realm, bytes.NewReader(d))
		if err != nil {
			t.Fatal(err)
		}
		blobs[i] = protocol.CacheBlob{Path: "p", SHA256: sha, Size: size}
	}
	l.b.Plan.Save = nil
	l.b.Plan.Restore = []protocol.CacheRestore{{Spec: "go", Key: restoreEntryKey, Dir: "go", Blobs: blobs[:]}}
	return l, s, blobs, datas
}

func TestRestoreBlobsListsOnlyThePlan(t *testing.T) {
	l, _, blobs, datas := restoreFixture(t)
	l.b.Plan.Restore = append(l.b.Plan.Restore, protocol.CacheRestore{Spec: "go", Key: strings.Repeat("d", 64), Dir: "go",
		Blobs: []protocol.CacheBlob{blobs[0], {Path: "z", SHA256: strings.Repeat("0", 64), Size: 0}}}) // a duplicate and an empty blob
	list, err := l.RestoreBlobs(context.Background())
	if err != nil || len(list) != 2 {
		t.Fatalf("list=%d err=%v", len(list), err)
	}
	for i, b := range list {
		rc, err := b.Open()
		if err != nil {
			t.Fatal(err)
		}
		got, _ := io.ReadAll(rc)
		_ = rc.Close()
		if !bytes.Equal(got, datas[i]) {
			t.Fatalf("blob %d differs from the stored bytes", i)
		}
	}
}

func TestRestoreBlobOpenNeedsACurrentEpoch(t *testing.T) {
	l, _, _, _ := restoreFixture(t)
	list, _ := l.RestoreBlobs(context.Background())
	l.b.Epoch++ // the lease moved on after the list was made
	if _, err := list[0].Open(); err == nil {
		t.Fatal("a blob opened under a stale epoch")
	}
}

func TestRestoreBlobOpenRefusesASizeTheStoreDoesNotHold(t *testing.T) {
	l, _, _, _ := restoreFixture(t)
	l.b.Plan.Restore[0].Blobs[0].Size += 5
	list, _ := l.RestoreBlobs(context.Background())
	if _, err := list[0].Open(); err == nil {
		t.Fatal("a blob opened at a size the store does not hold")
	}
}

func TestRestoreRefusalsAreAudited(t *testing.T) {
	l, s, _, _ := restoreFixture(t)
	for _, reason := range []string{"not_planned", "stale_epoch", "cancelled", "bad_offset"} {
		l.RestoreEvent(context.Background(), transport.RestoreEvent{Kind: transport.RestoreRefused, SHA256: strings.Repeat("e", 64), Reason: reason})
		assertAudit(t, s, "restore_"+reason)
	}
}

func TestRestoreLocalityOnlyAfterTheNodeConfirmedEveryBlob(t *testing.T) {
	l, s, blobs, _ := restoreFixture(t)
	ctx := context.Background()
	rows := func() []store.CacheLocalityRow {
		r, err := s.CacheLocality(ctx, "node", l.b.Plan.Namespace, restoreEntryKey)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	l.RestoreEvent(ctx, transport.RestoreEvent{Kind: transport.RestoreFailed, SHA256: blobs[0].SHA256, Reason: "gave up"})
	l.RestoreEvent(ctx, transport.RestoreEvent{Kind: transport.RestoreCopied, SHA256: blobs[0].SHA256, Bytes: blobs[0].Size})
	if len(rows()) != 0 {
		t.Fatal("a half confirmed entry was recorded as a node copy")
	}
	l.RestoreEvent(ctx, transport.RestoreEvent{Kind: transport.RestoreCopied, SHA256: blobs[1].SHA256, Bytes: blobs[1].Size})
	got := rows()
	if len(got) != 1 || got[0].Bytes != blobs[0].Size+blobs[1].Size {
		t.Fatalf("locality %+v", got)
	}
}

func TestRestoreLocalityIsNotWrittenUnderAStaleEpoch(t *testing.T) {
	l, s, blobs, _ := restoreFixture(t)
	l.b.Plan.Restore[0].Blobs = blobs[:1]
	l.b.Epoch++
	l.RestoreEvent(context.Background(), transport.RestoreEvent{Kind: transport.RestoreCopied, SHA256: blobs[0].SHA256, Bytes: blobs[0].Size})
	if r, _ := s.CacheLocality(context.Background(), "node", l.b.Plan.Namespace, restoreEntryKey); len(r) != 0 {
		t.Fatalf("a zombie lease wrote locality: %+v", r)
	}
	assertAudit(t, s, "stale_epoch")
}
