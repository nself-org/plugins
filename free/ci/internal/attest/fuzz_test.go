package attest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
)

// FuzzVerifyDocument: Verify never panics, and a document it accepts is one
// GateAccept's consistency check also finds well formed.
func FuzzVerifyDocument(f *testing.F) {
	attest.RegisterRemoteVerifier(nil)
	f.Add(marshal(f, goodEvidence()))
	f.Add(marshal(f, signedRemote(goodEvidence())))
	f.Add(marshal(f, leaky()))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"schema":"ci.evidence/v1"}`))
	files, _ := filepath.Glob("testdata/adversarial/*.json")
	for _, name := range files {
		if b, err := os.ReadFile(name); err == nil {
			f.Add(b)
		}
	}
	f.Fuzz(func(t *testing.T, doc []byte) {
		ev, err := attest.Verify(doc)
		if err != nil {
			if _, ok := err.(*attest.Rejection); !ok {
				t.Fatalf("error is not a *Rejection: %T", err)
			}
			return
		}
		if reasons := attest.Consistent(ev); len(reasons) != 0 {
			t.Fatalf("accepted a document that is not consistent: %v", reasons)
		}
		if ev.Producer.Kind == "signed-remote" || ev.Signature.Kind == "runner-ed25519" {
			t.Fatal("accepted a runner signature with no verifier registered")
		}
		if ev.Binding.State == "bound" && ev.WorktreeState != "clean" {
			t.Fatal("accepted a bound dirty document")
		}
	})
}
