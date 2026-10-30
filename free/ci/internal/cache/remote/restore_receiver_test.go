package remote

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

type auditRows struct {
	rows []string
	err  error
}

func (a *auditRows) CacheAuditOnce(_ context.Context, action, target, detail string) error {
	a.rows = append(a.rows, action+" "+target+" "+detail)
	return a.err
}

func restoreFixture(t *testing.T, payloads ...string) (*RestoreReceiver, *ReadThrough, *auditRows, protocol.Lease) {
	t.Helper()
	c, err := NewReadThrough(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	ns := authz.Namespace{Project: "project", Trust: "owner", Protection: authz.Protected}
	restore := protocol.CacheRestore{Spec: "go", Key: strings.Repeat("a", 64), Dir: "go/mod"}
	for i, p := range payloads {
		restore.Blobs = append(restore.Blobs, protocol.CacheBlob{Path: fmt.Sprint(i), SHA256: sum(p), Size: int64(len(p))})
	}
	lease := protocol.Lease{LeaseID: "lease-r", AttemptID: "attempt-r", Epoch: 4, Cache: &protocol.LeaseCache{
		Namespace: base64.RawURLEncoding.EncodeToString(ns.Canonical()), Access: "ro", Restore: []protocol.CacheRestore{restore}}}
	audit := &auditRows{}
	r, err := NewRestoreReceiver(c, lease, audit)
	if err != nil {
		t.Fatal(err)
	}
	return r, c, audit, lease
}

func sum(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }

func chunk(digest string, off int64, data string, final bool) protocol.Chunk {
	return protocol.Chunk{Stream: StreamCache, Offset: off, Data: base64.StdEncoding.EncodeToString([]byte(data)), Digest: digest, Final: final}
}

func wantReason(t *testing.T, err error, reason string) {
	t.Helper()
	var ref *entries.Refused
	if !errors.As(err, &ref) || ref.Reason != reason {
		t.Fatalf("want refusal %s, got %v", reason, err)
	}
}

func spools(t *testing.T, c *ReadThrough) int {
	t.Helper()
	list, err := os.ReadDir(c.dir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range list {
		if strings.HasPrefix(e.Name(), ".restore-") {
			n++
		}
	}
	return n
}

func TestRestoreReceiverWritesVerifiedCopy(t *testing.T) {
	ctx := context.Background()
	data := "restored cache bytes"
	r, c, audit, lease := restoreFixture(t, data)
	d := sum(data)
	if err := r.Digest(ctx, 4, protocol.Digest{Stream: StreamCache, SHA256: d, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if err := r.Chunk(ctx, 4, chunk(d, 0, data[:8], false)); err != nil {
		t.Fatal(err)
	}
	if err := r.Chunk(ctx, 4, chunk(d, 8, data[8:], true)); err != nil {
		t.Fatal(err)
	}
	if !r.Received(d) || len(audit.rows) != 0 || spools(t, c) != 0 {
		t.Fatalf("received=%v audit=%v spools=%d", r.Received(d), audit.rows, spools(t, c))
	}
	f, err := c.Open(ctx, lease.Cache.Namespace, d, int64(len(data)), nil)
	if err != nil {
		t.Fatalf("copy not published in lease realm: %v", err)
	}
	f.Close()
	if _, err := c.Open(ctx, "other-realm", d, int64(len(data)), nil); !os.IsNotExist(err) {
		t.Fatalf("copy leaked into another realm: %v", err)
	}
}

func TestRestoreReceiverDiscardsCorruptCopy(t *testing.T) {
	ctx := context.Background()
	data := "genuine bytes"
	r, c, audit, lease := restoreFixture(t, data)
	d := sum(data)
	if err := r.Digest(ctx, 4, protocol.Digest{Stream: StreamCache, SHA256: d, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	err := r.Chunk(ctx, 4, chunk(d, 0, strings.Repeat("x", len(data)), true))
	wantReason(t, err, entries.ReasonBlobInvalid)
	if r.Received(d) || len(audit.rows) != 1 || spools(t, c) != 0 {
		t.Fatalf("corrupt copy kept: audit=%v spools=%d", audit.rows, spools(t, c))
	}
	if _, err := c.Open(ctx, lease.Cache.Namespace, d, int64(len(data)), nil); !os.IsNotExist(err) {
		t.Fatalf("corrupt copy published: %v", err)
	}
}

func TestRestoreReceiverRefusesStaleEpoch(t *testing.T) {
	ctx := context.Background()
	data := "bytes"
	r, c, audit, _ := restoreFixture(t, data)
	d := sum(data)
	wantReason(t, r.Digest(ctx, 3, protocol.Digest{Stream: StreamCache, SHA256: d, Size: 5}), entries.ReasonEpoch)
	if err := r.Digest(ctx, 4, protocol.Digest{Stream: StreamCache, SHA256: d, Size: 5}); err != nil {
		t.Fatal(err)
	}
	wantReason(t, r.Chunk(ctx, 5, chunk(d, 0, data, true)), entries.ReasonEpoch)
	if r.Received(d) || spools(t, c) != 0 || len(audit.rows) != 2 || !strings.Contains(audit.rows[0], AuditRestoreRefused+" lease-r/"+d+" epoch") {
		t.Fatalf("stale epoch: audit=%v spools=%d", audit.rows, spools(t, c))
	}
	// The partial copy was dropped: a resumed chunk without a new announcement fails.
	wantReason(t, r.Chunk(ctx, 4, chunk(d, 0, data, true)), entries.ReasonBlobInvalid)
}

func TestRestoreReceiverRefusesAfterCancel(t *testing.T) {
	ctx := context.Background()
	data := "bytes"
	r, c, audit, _ := restoreFixture(t, data)
	d := sum(data)
	if err := r.Digest(ctx, 4, protocol.Digest{Stream: StreamCache, SHA256: d, Size: 5}); err != nil {
		t.Fatal(err)
	}
	r.Cancel()
	if spools(t, c) != 0 {
		t.Fatal("cancel kept the partial copy")
	}
	wantReason(t, r.Chunk(ctx, 4, chunk(d, 0, data, true)), ReasonLeaseCancelled)
	wantReason(t, r.Digest(ctx, 4, protocol.Digest{Stream: StreamCache, SHA256: d, Size: 5}), ReasonLeaseCancelled)
	if r.Received(d) || len(audit.rows) != 2 || !strings.HasSuffix(audit.rows[0], ReasonLeaseCancelled) {
		t.Fatalf("cancel audit=%v", audit.rows)
	}
}

func TestRestoreReceiverRefusesUnplannedDigests(t *testing.T) {
	ctx := context.Background()
	r, c, audit, _ := restoreFixture(t, "planned")
	other := sum("unplanned")
	wantReason(t, r.Digest(ctx, 4, protocol.Digest{Stream: StreamCache, SHA256: other, Size: 9}), entries.ReasonNotPlanned)
	wantReason(t, r.Chunk(ctx, 4, chunk(other, 0, "unplanned", true)), entries.ReasonNotPlanned)
	d := sum("planned")
	wantReason(t, r.Digest(ctx, 4, protocol.Digest{Stream: "artifact", SHA256: d, Size: 7}), entries.ReasonNotPlanned)
	wantReason(t, r.Digest(ctx, 4, protocol.Digest{Stream: StreamCache, SHA256: d, Size: 8}), entries.ReasonBlobInvalid)
	if len(audit.rows) != 4 || spools(t, c) != 0 {
		t.Fatalf("unplanned audit=%v spools=%d", audit.rows, spools(t, c))
	}
}

func TestRestoreReceiverRejectsOversizeAndGaps(t *testing.T) {
	ctx := context.Background()
	data := "sized"
	r, c, _, _ := restoreFixture(t, data)
	d := sum(data)
	announce := protocol.Digest{Stream: StreamCache, SHA256: d, Size: 5}
	if err := r.Digest(ctx, 4, announce); err != nil {
		t.Fatal(err)
	}
	wantReason(t, r.Chunk(ctx, 4, chunk(d, 0, "sized+extra", false)), entries.ReasonSizeCap)
	if err := r.Digest(ctx, 4, announce); err != nil {
		t.Fatal(err)
	}
	wantReason(t, r.Chunk(ctx, 4, chunk(d, 2, "zed", true)), entries.ReasonBlobInvalid)
	if err := r.Digest(ctx, 4, announce); err != nil {
		t.Fatal(err)
	}
	wantReason(t, r.Chunk(ctx, 4, chunk(d, 0, "siz", true)), entries.ReasonBlobInvalid)
	if spools(t, c) != 0 || r.Received(d) {
		t.Fatal("rejected stream left a copy")
	}
}

func TestRestoreReceiverSurfacesAuditFailure(t *testing.T) {
	r, _, audit, _ := restoreFixture(t, "x")
	audit.err = errors.New("ledger down")
	err := r.Digest(context.Background(), 9, protocol.Digest{Stream: StreamCache, SHA256: sum("x"), Size: 1})
	wantReason(t, err, entries.ReasonEpoch)
	if !strings.Contains(err.Error(), "ledger down") {
		t.Fatalf("audit failure hidden: %v", err)
	}
}

func TestNewRestoreReceiverValidatesLease(t *testing.T) {
	c, err := NewReadThrough(t.TempDir(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRestoreReceiver(c, protocol.Lease{LeaseID: "l"}, nil); err == nil {
		t.Fatal("lease without cache plan accepted")
	}
	bad := protocol.Lease{LeaseID: "l", Cache: &protocol.LeaseCache{Namespace: "!!"}}
	_, err = NewRestoreReceiver(c, bad, nil)
	wantReason(t, err, entries.ReasonNamespace)
}
