package attest_test

// Purpose: the result-frame signature check (D-1103): a frame the node signed the way the agent does
// verifies; a changed document, a wrong signer and a missing signature are each refused with their reason.

import (
	"encoding/base64"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
)

func TestVerifyFrame(t *testing.T) {
	ev := attemptEvidence("att-1")
	ev.Producer.Kind = "self-reported" // what a node's document says before the coordinator accepts the signature
	doc := marshal(t, ev)
	sign := func(d []byte, s seedSigner) (string, string) {
		raw, err := attest.SignDocumentRaw(d, s)
		if err != nil {
			t.Fatal(err)
		}
		return attest.EvidenceKeyID(s.KeyID()), base64.RawStdEncoding.EncodeToString(raw)
	}
	keyID, value := sign(doc, keyA)
	otherKey, otherValue := sign(doc, keyB)
	changed := ev
	changed.Revision = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	tampered := marshal(t, changed)
	for _, c := range []struct {
		name     string
		doc      []byte
		key, sig string
		ledger   func(*memLedger)
		reason   attest.RemoteReason
	}{
		{name: "valid", doc: doc, key: keyID, sig: value},
		{name: "changed document", doc: tampered, key: keyID, sig: value, reason: attest.RemoteRevisionMismatch},
		{name: "signed by a node that did not hold the lease", doc: doc, key: otherKey, sig: otherValue, reason: attest.RemoteLeaseHolderMismatch},
		{name: "revoked key", doc: doc, key: keyID, sig: value, ledger: func(l *memLedger) { l.revoked[keyID] = true }, reason: attest.RemoteKeyRevoked},
		{name: "no key id", doc: doc, sig: value, reason: attest.RemoteSignatureMissing},
		{name: "no signature", doc: doc, key: keyID, reason: attest.RemoteSignatureMissing},
		{name: "not base64", doc: doc, key: keyID, sig: "not base64!", reason: attest.RemoteSignatureMalformed},
	} {
		l := newLedger()
		if c.ledger != nil {
			c.ledger(l)
		}
		err := attest.VerifyFrame(func() attest.Ledger { return l }, c.doc, c.key, c.sig)
		if got := attest.RemoteReasonOf(err); got != c.reason {
			t.Errorf("%s: reason %q (%v), want %q", c.name, got, err, c.reason)
		}
	}
	if err := attest.VerifyFrame(func() attest.Ledger { return nil }, doc, keyID, value); attest.RemoteReasonOf(err) != attest.RemoteLedgerUnavailable {
		t.Errorf("no ledger: %v, want ledger_unavailable", err)
	}
}
