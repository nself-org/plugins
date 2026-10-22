package runtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers"
)

func TestLivenessLeaseEpochBinding(t *testing.T) {
	useProvider(t)
	ctx := context.Background()
	book := NewMemBook()
	if _, err := book.Put(ctx, "a", Entry{Epoch: 2, Handle: providers.Handle{Provider: "fake"}}); err != nil {
		t.Fatal(err)
	}
	for _, epoch := range []int64{1, 2, 3} {
		got, err := liveness("fake", prov, book)(ctx, "a", epoch)
		if err != nil {
			t.Fatal(err)
		}
		if (got == lease.LiveVanished) != (epoch != 2) {
			t.Fatalf("epoch=%d state=%s", epoch, got)
		}
	}
}

func TestEvidenceAttestationBinding(t *testing.T) {
	ev, _ := json.Marshal(model.Evidence{Schema: "ci.evidence/v1", Revision: testRev, Attempt: &model.AttemptRef{ID: "a", N: 2}})
	l := protocol.Lease{AttemptID: "a", Checkout: protocol.Checkout{Revision: testRev}}
	h := providers.Handle{Provider: "fake", RunID: "run", Nonce: "nonce"}
	for _, tc := range []struct {
		name, ref string
		n         int
		wantErr   bool
	}{
		{"valid", testRev, 2, false}, {"foreign ref", "other", 2, true}, {"foreign attempt", testRev, 1, true}, {"missing attempt", testRev, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := providers.Collected{Evidence: ev, Attestation: model.ProviderAttestation{Provider: h.Provider, RunID: h.RunID, Nonce: h.Nonce, Ref: tc.ref, RunAttempt: tc.n}}
			if _, err := bindEvidence(c, h, l, 2, time.Now()); (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}
