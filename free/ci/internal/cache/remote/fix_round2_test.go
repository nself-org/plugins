package remote

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

func TestCacheRemoteSaveCannotRestoreUntrustedSnapshot(t *testing.T) {
	l, s, e := coordinatorFixture(t)
	l.h.Entries = entries.New(s, l.h.Blobs, entries.Options{DisableFloor: true, Project: func(string) authz.ProjectFacts { return authz.ProjectFacts{NoPrivateDeps: true} }})
	addRef(t, l, l.b.Plan.Save[0].Key, cacheArchive(t, "payload", false))
	if err := e.Result(context.Background(), "l", l.b.Epoch); err != nil {
		t.Fatal(err)
	}
	if err := l.Finalize(context.Background(), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	ns, _ := DecodePlanNamespace(l.b.Plan.Namespace)
	if _, hit, err := l.h.Entries.LookupForRestore(context.Background(), ns, l.b.Plan.Save[0].Key, authz.ModeROSnapshot); err != nil || hit {
		t.Fatalf("untrusted snapshot hit=%v err=%v", hit, err)
	}
	rows, err := s.CacheEntries(context.Background(), "")
	if err != nil || len(rows) != 1 || !rows[0].Writer.PrivateDeps {
		t.Fatalf("remote private-deps not tightened: %v %v", rows, err)
	}
}

func TestCacheFinalizeCargoGitArchiveAccepted(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		t.Run(fmt.Sprint(compressed), func(t *testing.T) {
			l, s, e := coordinatorFixture(t)
			addRef(t, l, l.b.Plan.Save[0].Key, cacheArchive(t, "checkouts/dependency/.git/config", compressed))
			if err := e.Result(context.Background(), "l", l.b.Epoch); err != nil {
				t.Fatal(err)
			}
			if err := l.Finalize(context.Background(), time.Millisecond); err != nil {
				t.Fatal(err)
			}
			if rows, err := s.CacheEntries(context.Background(), ""); err != nil || len(rows) != 1 {
				t.Fatalf("entries=%v err=%v", rows, err)
			}
		})
	}
}

type deadlineLedger struct {
	Ledger
	remaining time.Duration
}

func (d *deadlineLedger) LeaseGet(ctx context.Context, id string) (store.RemoteLease, error) {
	if deadline, ok := ctx.Deadline(); ok {
		d.remaining = time.Until(deadline)
	}
	return d.Ledger.LeaseGet(ctx, id)
}

func TestCacheFinalizeFreshCommitBudgetAfterWait(t *testing.T) {
	l, s, e := coordinatorFixture(t)
	l.b.Unordered = true
	l.b.Plan.Save = append(l.b.Plan.Save, protocol.CacheSave{Spec: "go", Key: strings.Repeat("b", 64), Dirs: []string{"go/mod"}})
	addRef(t, l, l.b.Plan.Save[0].Key, cacheArchive(t, "payload", false))
	if err := e.Result(context.Background(), "l", l.b.Epoch); err != nil {
		t.Fatal(err)
	}
	d := &deadlineLedger{Ledger: s}
	l.h.Ledger = d
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	if err := l.Finalize(ctx, 200*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if d.remaining < 30*time.Second {
		t.Fatalf("commit inherited spent wait deadline: %s", d.remaining)
	}
}

func TestCacheArchiveValidationDoesNotCreateSnapshot(t *testing.T) {
	l, s, _ := coordinatorFixture(t)
	home := t.TempDir()
	b, err := cas.New(home)
	if err != nil {
		t.Fatal(err)
	}
	l.h.Blobs = b
	l.h.Entries = entries.New(s, b, entries.Options{DisableFloor: true})
	addRef(t, l, l.b.Plan.Save[0].Key, cacheArchive(t, "payload", false))
	// Snapshot creation would need this directory; streaming validation needs none.
	tmp := filepath.Join(home, "blobs", "tmp")
	if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte("snapshot unavailable"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := l.validateRef(context.Background(), l.refs[l.b.Plan.Save[0].Key]); err != nil {
		t.Fatal(err)
	}
}

func TestCacheReceiverDistinctBlobCapAndDedupe(t *testing.T) {
	l, _, _ := coordinatorFixture(t)
	ctx := context.Background()
	hdr := func(s string) transport.BlobHeader {
		return transport.BlobHeader{Stream: StreamCache, SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(s))), Size: int64(len(s))}
	}
	// Realm hits do not consume the new-blob count, even across many digests.
	for _, data := range []string{"a", "b", "c"} {
		if _, _, err := l.h.Blobs.Put(ctx, cacheRealm(l.b.Plan.Namespace), bytes.NewBufferString(data)); err != nil {
			t.Fatal(err)
		}
		w, err := l.Blob(ctx, hdr(data))
		if err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}
	w, err := l.Blob(ctx, hdr("new"))
	if err != nil {
		t.Fatal(err)
	}
	defer func(cleanup func() error) { _ = cleanup() }(w.Close)
	if len(l.held) != 1 {
		t.Fatalf("dedupe charged blob count: %v", l.held)
	}
	if _, err := l.Blob(ctx, hdr("extra")); !transfer.IsReason(err, transfer.ReasonSizeCap) {
		t.Fatalf("distinct digest accepted: %v", err)
	}
	if again, err := l.Blob(ctx, hdr("new")); err != nil {
		t.Fatalf("reannounce charged twice: %v", err)
	} else {
		_ = again.Close()
	}
}

func TestCacheRetractDropsAllEntryLocality(t *testing.T) {
	l, s, e := coordinatorFixture(t)
	l.b.Node = "node-a"
	ctx := context.Background()
	addRef(t, l, l.b.Plan.Save[0].Key, cacheArchive(t, "payload", false))
	if err := e.Result(ctx, "l", l.b.Epoch); err != nil {
		t.Fatal(err)
	}
	if err := l.Finalize(ctx, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := s.CacheLocalityPut(ctx, store.CacheLocalityRow{Node: "node-b", Realm: l.b.Plan.Namespace, Key: l.b.Plan.Save[0].Key, Bytes: 1}); err != nil {
		t.Fatal(err)
	}
	if err := l.RetractUsing(ctx, s); err != nil {
		t.Fatal(err)
	}
	if rows, err := s.CacheLocality(ctx, "", l.b.Plan.Namespace, l.b.Plan.Save[0].Key); err != nil || len(rows) != 0 {
		t.Fatalf("stale locality=%v err=%v", rows, err)
	}
}
