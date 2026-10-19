package signing

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/secrets"
)

// The signer's two refs must go through the secrets grammar, not a hand-built Ref (D-1096).
func TestKeyRefsParseAsSignerRefs(t *testing.T) {
	for _, raw := range []string{KeyRef, KeyIDRef} {
		want, err := secrets.ParseSigner(raw)
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		got, err := releaseRef(raw)
		if err != nil || got != want {
			t.Errorf("%s: releaseRef = %+v, %v; want %+v", raw, got, err, want)
		}
	}
	if _, err := releaseRef("release/PATH"); err == nil {
		t.Error("a reserved non-NSELF_CI_ name must still be refused")
	}
}
