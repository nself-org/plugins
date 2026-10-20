package remote

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

func data(seed byte, n int) []byte {
	b := pattern(n)
	b[0] = seed
	return b
}

func TestInterleavedBlobsAndRefsAreChargedOnce(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, jobWith("out/*"))
	r.h.Policy.MaxTotal = 10_000
	var all [][]byte
	for i := 0; i < 3; i++ { // 3 x 3000 B under a 10000 B cap, a ref after each blob, as Ship sends them
		d := data(byte(i+1), 3000)
		all = append(all, d)
		if err := r.upload(d); err != nil {
			t.Fatalf("blob %d: %v", i+1, err)
		}
		if err := r.lease.ArtifactRef(ctx, ref("out/f"+string(rune('a'+i)), d)); err != nil {
			t.Fatalf("ref %d: %v", i+1, err)
		}
	}
	if err := r.upload(data(9, 1500)); !transfer.IsReason(err, transfer.ReasonSizeCap) {
		t.Fatalf("a 4th blob of 1500 B does not fit in the 1000 B left: %v", err)
	}
	_ = all
}

func TestResumeAndDuplicateAnnouncesAreNotChargedTwice(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, jobWith("out/*"))
	r.h.Policy.MaxTotal = 10_000
	big := data(1, 6000)
	h := transport.BlobHeader{Stream: "artifact", SHA256: sum(big), Size: int64(len(big))}
	w, err := r.lease.Blob(ctx, h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write(big[:2000]); err != nil {
		t.Fatal(err)
	}
	_ = w.(transport.BlobWriter)
	if ab, ok := w.(interface{ Abort() error }); ok {
		_ = ab.Abort()
	}
	w2, err := r.lease.Blob(ctx, h) // the sender re-announces after a cut: 6000 B again against 10000 B
	if err != nil {
		t.Fatalf("re-announce: %v", err)
	}
	if w2.Offset() != 2000 {
		t.Fatalf("offset after the cut = %d, want 2000", w2.Offset())
	}
	if _, err = w2.Write(big[2000:]); err != nil {
		t.Fatal(err)
	}
	if err = w2.Close(); err != nil {
		t.Fatal(err)
	}
	// announced once more after it is complete: held, free, no second reservation
	w3, err := r.lease.Blob(ctx, h)
	if err != nil || w3.Offset() != int64(len(big)) {
		t.Fatalf("duplicate announce: offset %d err %v", w3.Offset(), err)
	}
	if err = r.lease.ArtifactRef(ctx, ref("out/a", big)); err != nil {
		t.Fatal(err)
	}
	if err = r.upload(data(2, 3500)); err != nil { // 6000 recorded + 3500 = 9500 <= 10000
		t.Fatalf("a blob that fits: %v", err)
	}
}

func TestStoreFaultIsNotReportedOrAuditedAsAStaleEpoch(t *testing.T) {
	r := newRig(t, jobWith("out/a"))
	boom := errors.New("database is locked")
	l := r.h.Bind(Binding{AttemptID: "a1", LeaseID: "lease-a1", Epoch: 1, Job: jobWith("out/a"), Fence: func(context.Context) error { return boom }})
	d := pattern(100)
	_, err := l.Blob(context.Background(), transport.BlobHeader{Stream: "artifact", SHA256: sum(d), Size: 100})
	if err == nil || transfer.IsReason(err, transfer.ReasonStaleEpoch) {
		t.Fatalf("err = %v, want the fault as it is", err)
	}
	if n := r.audits("artifact.remote.refused"); n != 0 {
		t.Fatalf("audit rows = %d for a transient fault", n)
	}
}

func TestEpochIsReadBetweenTwoFences(t *testing.T) {
	var fences atomic.Int32
	r := newRig(t, jobWith("out/a"))
	orig := r.lease.b
	r.lease = r.h.Bind(Binding{AttemptID: "a1", LeaseID: "lease-a1", Epoch: orig.Epoch, Job: orig.Job, Ready: orig.Ready,
		Fence: func(ctx context.Context) error { fences.Add(1); return orig.Fence(ctx) }})
	if err := r.lease.Declare(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fences.Load() < 2 { // live, then the second fence after the epoch read
		t.Fatalf("fenced %d times", fences.Load())
	}
}

func TestAnotherLeaseObjectOfTheSameEpochAdoptsTheDeclarations(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, jobWith("out/a", "out/b"))
	if err := r.lease.Declare(ctx); err != nil {
		t.Fatal(err)
	}
	second := r.h.Bind(r.lease.b)
	if err := second.Declare(ctx); err != nil {
		t.Fatalf("a second lease object for the same delivery: %v", err)
	}
	decls, _ := r.st.ArtifactDecls(ctx, "a1")
	if len(decls) != 2 {
		t.Fatalf("declarations = %d", len(decls))
	}
	var _ records.Decl
	_ = protocol.ArtifactRef{}
}

func TestManyRefsToOneBlobAreEachChargedAgainstTheStore(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, jobWith("out/*"))
	r.h.Policy.MaxTotal = 10_000
	d := data(7, 3000)
	if err := r.upload(d); err != nil {
		t.Fatal(err)
	}
	var ok int
	var last error
	for i := 0; i < 10; i++ {
		last = r.lease.ArtifactRef(ctx, ref("out/f"+string(rune('a'+i)), d))
		if last != nil {
			break
		}
		ok++
	}
	if ok != 3 || !transfer.IsReason(last, transfer.ReasonSizeCap) {
		t.Fatalf("%d refs of 3000 B accepted under a 10000 B cap (last error %v), want 3 then E714 size_cap", ok, last)
	}
	u, _ := r.st.ArtifactUsage(ctx, "proj")
	if u.Total > 10_000 {
		t.Fatalf("store usage %d B above the cap", u.Total)
	}
}

func TestSelectIsPolynomialInTheNumberOfDoubleStars(t *testing.T) {
	deep := strings.Repeat("d/", 200) + "x"
	start := time.Now()
	if selects("**/**/**/**/**/**/y", deep) {
		t.Fatal("no file y")
	}
	if !selects("**/**/**/**/x", deep) {
		t.Fatal("x is selected")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("took %s", time.Since(start))
	}
	if selects("a", strings.Repeat("a/", maxComponents+1)+"b") {
		t.Fatal("a path deeper than the bound selects nothing")
	}
}
