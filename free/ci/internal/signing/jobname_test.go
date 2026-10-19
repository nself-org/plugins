package signing_test

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
	"github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// The evidence and `artifacts verify` call the release job by its plan id (the
// job row's name); the signer must accept that spelling, or no real run could be
// published (P7-CACHE-18). The row id still works, and a spelling that matches
// two jobs is refused.
func TestManifestMayNameTheReleaseJobByItsPlanID(t *testing.T) {
	for _, id := range []string{"job-rel", "publish"} {
		f := newFixture(t)
		req := request(item(manifest.PayloadType, f.manifestPayload(func(m *manifest.Manifest) { m.Release.JobID = id })))
		if _, err := f.handler().Handle(context.Background(), req, decodeAll(t, req)); err != nil {
			t.Fatalf("job %q: %v", id, err)
		}
	}
	f := newFixture(t)
	f.src.jobs = append(f.src.jobs, store.RunJob{ID: "other-row", Name: "job-rel", Kind: "release"})
	req := request(item(manifest.PayloadType, f.manifestPayload(nil)))
	_, err := f.handler().Handle(context.Background(), req, decodeAll(t, req))
	if !signing.IsReason(err, signing.ReasonPayloadRejected) {
		t.Fatalf("an ambiguous job name was accepted: %v", err)
	}
	f = newFixture(t)
	req = request(item(manifest.PayloadType, f.manifestPayload(func(m *manifest.Manifest) { m.Release.JobID = "nope" })))
	if _, err = f.handler().Handle(context.Background(), req, decodeAll(t, req)); !signing.IsReason(err, signing.ReasonPayloadRejected) {
		t.Fatalf("an unknown job was accepted: %v", err)
	}
}
