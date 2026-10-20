package remote

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func ref(path string, data []byte) protocol.ArtifactRef {
	return protocol.ArtifactRef{LeaseID: "lease-a1", Path: path, SHA256: sum(data), Size: int64(len(data))}
}

func TestDeliveredArtifactCommitsAndVerifies(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, jobWith("out/*.txt"))
	data := pattern(300_000)
	if err := r.upload(data); err != nil {
		t.Fatal(err)
	}
	if err := r.lease.ArtifactRef(ctx, ref("out/a.txt", data)); err != nil {
		t.Fatal(err)
	}
	_, epoch, _ := r.st.AttemptStateEpoch(ctx, "a1")
	if err := r.st.Transition(ctx, "a1", "running", "finalizing", store.TransitionDetail{Epoch: epoch}); err != nil {
		t.Fatal(err)
	}
	_, epoch, _ = r.st.AttemptStateEpoch(ctx, "a1")
	got, err := records.CommitAndVerify(ctx, r.st, r.blobs, "a1", epoch, r.pol)
	if err != nil || len(got) != 1 {
		t.Fatalf("commit: %v %v", got, err)
	}
	rec := got[0]
	if rec.Name != "out/a.txt" || rec.SHA256 != sum(data) || rec.Size != int64(len(data)) || rec.Project != "proj" || rec.State != records.StateCommitted || rec.Revision != "rev" {
		t.Fatalf("record = %+v", rec)
	}
}

func TestRefForAPathNoDeclarationSelectsIsRefused(t *testing.T) {
	r := newRig(t, jobWith("out/*.txt"))
	data := pattern(100)
	if err := r.upload(data); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"other/a.txt", "out/sub/a.go", "../out/a.txt", "/etc/passwd", "out\\a.txt", ""} {
		if err := r.lease.ArtifactRef(context.Background(), ref(p, data)); !records.IsReason(err, ReasonNotDeclared) {
			t.Errorf("ref %q: %v, want E712 not_declared", p, err)
		}
	}
	staged, _ := r.st.ArtifactStaged(context.Background(), "a1")
	if len(staged) != 0 {
		t.Fatalf("staged = %v", staged)
	}
}

func TestRefNeedsTheBlobAtTheStatedSize(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, jobWith("out/a"))
	data := pattern(100)
	if err := r.lease.ArtifactRef(ctx, ref("out/a", data)); !records.IsReason(err, records.ReasonMissingBlob) {
		t.Fatalf("never delivered: %v", err)
	}
	if err := r.upload(data); err != nil {
		t.Fatal(err)
	}
	bad := ref("out/a", data)
	bad.Size = 99
	if err := r.lease.ArtifactRef(ctx, bad); !transfer.IsReason(err, transfer.ReasonDigest) {
		t.Fatalf("wrong size: %v", err)
	}
	if err := r.lease.ArtifactRef(ctx, ref("out/a", data)); err != nil {
		t.Fatal(err)
	}
	if err := r.lease.ArtifactRef(ctx, ref("out/a", data)); !records.IsReason(err, records.ReasonDuplicate) {
		t.Fatalf("second ref for the same name: %v", err)
	}
	wrong := ref("out/a", data)
	wrong.LeaseID = "lease-other"
	if err := r.lease.ArtifactRef(ctx, wrong); !transfer.IsReason(err, transfer.ReasonProtocol) {
		t.Fatalf("another lease: %v", err)
	}
}

func TestEmptyArtifactIsStoredByTheRef(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, jobWith("out/empty"))
	bad := ref("out/empty", nil)
	bad.SHA256 = sum([]byte("x"))
	if err := r.lease.ArtifactRef(ctx, bad); !transfer.IsReason(err, transfer.ReasonDigest) {
		t.Fatalf("empty file with another digest: %v", err)
	}
	if err := r.lease.ArtifactRef(ctx, ref("out/empty", nil)); err != nil {
		t.Fatal(err)
	}
}

func TestStaleEpochRefusesBlobsAndRefsWithOneAuditRowPerBlob(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, jobWith("out/a"))
	data := pattern(5000)
	r.expire()
	for i := 0; i < 3; i++ {
		err := r.upload(data)
		if !transfer.IsReason(err, transfer.ReasonStaleEpoch) {
			t.Fatalf("upload %d: %v, want E714 stale_epoch", i, err)
		}
	}
	if err := r.lease.ArtifactRef(ctx, ref("out/a", data)); !transfer.IsReason(err, transfer.ReasonStaleEpoch) {
		t.Fatalf("ref: %v", err)
	}
	if r.has(data) {
		t.Fatal("a zombie's blob was committed")
	}
	if n := r.audits("artifact.remote.refused"); n != 1 {
		t.Fatalf("audit rows = %d, want one for the blob", n)
	}
}

func TestAnnounceAboveCapsIsRefusedBeforeAnyChunk(t *testing.T) {
	r := newRig(t, jobWith("out/a"))
	r.h.Policy.MaxFile = 1000
	big := pattern(1001)
	if err := r.upload(big); !transfer.IsReason(err, transfer.ReasonSizeCap) {
		t.Fatalf("file cap: %v", err)
	}
	r2 := newRig(t, jobWith("out/a"))
	r2.h.Policy.MaxFile, r2.h.Policy.MaxTotal = 1<<20, 1500
	if err := r2.upload(pattern(1000)); err != nil {
		t.Fatal(err)
	}
	if err := r2.upload(pattern(900)); !transfer.IsReason(err, transfer.ReasonSizeCap) {
		t.Fatalf("lease byte budget: %v", err)
	}
}

func TestJobWithoutDeclarationsTakesNoArtifacts(t *testing.T) {
	r := newRig(t, jobWith())
	if err := r.lease.ArtifactRef(context.Background(), ref("out/a", pattern(10))); !records.IsReason(err, ReasonNotDeclared) {
		t.Fatalf("%v", err)
	}
}

func TestDeclareIsIdempotentAndFailsAnUndeliveredDeclaration(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, jobWith("out/a", "out/b"))
	for i := 0; i < 2; i++ {
		if err := r.lease.Declare(ctx); err != nil {
			t.Fatal(err)
		}
	}
	data := pattern(50)
	_ = r.upload(data)
	if err := r.lease.ArtifactRef(ctx, ref("out/a", data)); err != nil {
		t.Fatal(err)
	}
	_, epoch, _ := r.st.AttemptStateEpoch(ctx, "a1")
	_ = r.st.Transition(ctx, "a1", "running", "finalizing", store.TransitionDetail{Epoch: epoch})
	_, epoch, _ = r.st.AttemptStateEpoch(ctx, "a1")
	if _, err := records.CommitAndVerify(ctx, r.st, r.blobs, "a1", epoch, r.pol); !records.IsReason(err, records.ReasonMissing) || !strings.Contains(err.Error(), "out/b") {
		t.Fatalf("commit with out/b undelivered: %v", err)
	}
}
