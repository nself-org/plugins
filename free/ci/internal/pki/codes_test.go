package pki

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestErrorShapeAndReasonOf(t *testing.T) {
	err := fail(ReasonRevoked, "certificate of %s is revoked", "n1")
	if err.Error() != "E660: certificate of n1 is revoked (revoked)" {
		t.Fatalf("text %q", err.Error())
	}
	var e *Error
	if !errors.As(err, &e) || e.ErrCode() != "E660" || e.ErrReason() != "revoked" || CodeInvalid != "E660" {
		t.Fatalf("accessors: %+v", e)
	}
	if ReasonOf(err) != "revoked" || ReasonOf(fmt.Errorf("wrapped: %w", err)) != "revoked" {
		t.Fatal("ReasonOf must see through wrapping")
	}
	if ReasonOf(nil) != "" || ReasonOf(errors.New("plain")) != "" {
		t.Fatal("ReasonOf of a non-E660 error must be empty")
	}
	for _, p := range model.CodeProblems() {
		if strings.HasPrefix(p, "E660") {
			t.Fatalf("E660 registration problem: %s", p)
		}
	}
}

func TestReasonKeysAreDistinctAndStable(t *testing.T) {
	want := map[string]string{
		ReasonRevoked: "revoked", ReasonExpired: "expired", ReasonWrongKind: "wrong_kind", ReasonBadChain: "bad_chain",
		ReasonNoCert: "no_certificate", ReasonUnknownNode: "unknown_node", ReasonClockSkew: "clock_skew", ReasonCertTTL: "cert_ttl",
		ReasonRenewCap: "renew_cap", ReasonBadCSR: "bad_csr", ReasonNodeKeyChanged: "node_key_changed",
		ReasonFingerprintMismatch: "fingerprint_mismatch", ReasonFingerprintUnconfirmed: "fingerprint_unconfirmed",
		ReasonTokenSignature: "token_signature", ReasonTokenExpired: "token_expired", ReasonTokenUnknownKey: "token_unknown_key",
		ReasonTokenMalformed: "token_malformed",
	}
	if len(want) != 17 {
		t.Fatalf("reason keys collide: %d distinct", len(want))
	}
	for got, w := range want {
		if got != w {
			t.Errorf("reason %q, want %q", got, w)
		}
	}
}
