package attest_test

// Purpose: tests of runner-key signing (P7-TRUST-20): the payload covers the document minus signature and
// freshness with unknown fields and big integers kept, the signature is the node key's over it, and the
// key id carries the ci-node purpose.
// Inputs:  a real node key in a temp dir.
// Outputs: pass or fail; no network.
// Constraints: tests named TestSign* are the ones the Ticket's mutants run against.

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func newNodeKey(t testing.TB) *nodekey.Key {
	t.Helper()
	k, err := nodekey.LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestSignProducesRunnerSignature(t *testing.T) {
	key := newNodeKey(t)
	ev := goodEvidence()
	ev.Attempt = &model.AttemptRef{ID: "att-1", N: 1}
	signed, err := attest.Sign(ev, key)
	if err != nil {
		t.Fatal(err)
	}
	s := signed.Signature
	if signed.Producer.Kind != "signed-remote" || s.Kind != "runner-ed25519" || s.KeyID == nil || s.Value == nil {
		t.Fatalf("not a signed-remote document: %+v %+v", signed.Producer, s)
	}
	if want := "ci-node-" + key.KeyID(); *s.KeyID != want || len(*s.KeyID) != len("ci-node-")+16 {
		t.Fatalf("key id %q want %q", *s.KeyID, want)
	}
	raw, err := base64.RawStdEncoding.Strict().DecodeString(*s.Value)
	if err != nil || len(raw) != ed25519.SignatureSize {
		t.Fatalf("value is not unpadded base64 of a 64-byte signature: %v", err)
	}
	payload, err := attest.SigningPayload(marshal(t, signed))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(payload), "nself-ci-evidence/v1\n{") || !ed25519.Verify(key.Public(), payload, raw) {
		t.Fatalf("the signature does not verify over the payload: %.60s", payload)
	}
	if _, err := attest.Sign(ev, nil); err == nil {
		t.Fatal("signed without a signer")
	}
}

func TestSignPayloadExcludesSignatureAndFreshness(t *testing.T) {
	ev := goodEvidence()
	a := marshal(t, ev)
	ev.Freshness = model.Freshness{State: "stale"}
	ev.Signature = model.Signature{Kind: "runner-ed25519", KeyID: strp("ci-node-0123456789abcdef"), Value: strp("c2ln")}
	b := marshal(t, ev)
	pa, errA := attest.SigningPayload(a)
	pb, errB := attest.SigningPayload(b)
	if errA != nil || errB != nil {
		t.Fatal(errA, errB)
	}
	if string(pa) != string(pb) {
		t.Fatalf("signature or freshness changed the payload:\n%s\n%s", pa, pb)
	}
	for _, field := range []string{`"signature"`, `"freshness"`} {
		if strings.Contains(string(pa), field) {
			t.Fatalf("payload holds %s", field)
		}
	}
	ev.Result = "fail"
	pc, _ := attest.SigningPayload(marshal(t, ev))
	if string(pc) == string(pa) {
		t.Fatal("an edited result left the payload unchanged")
	}
}

func TestSignPayloadKeepsUnknownFieldsAndBigIntegers(t *testing.T) {
	doc := `{"b":1,"a":{"n":12345678901234567890,"z":[true,null,"x"]},"future_field":{"k":9007199254740993},"signature":{"kind":"runner-ed25519"},"freshness":{"state":"current"}}`
	got, err := attest.SigningPayload([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	want := `nself-ci-evidence/v1` + "\n" + `{"a":{"n":12345678901234567890,"z":[true,null,"x"]},"b":1,"future_field":{"k":9007199254740993}}`
	if string(got) != want {
		t.Fatalf("payload\n got %s\nwant %s", got, want)
	}
	reordered := `{"future_field":{"k":9007199254740993},"b":1,"a":{"z":[true,null,"x"],"n":12345678901234567890}}`
	again, err := attest.SigningPayload([]byte(reordered))
	if err != nil || string(again) != want {
		t.Fatalf("key order changed the payload: %v %s", err, again)
	}
}

func TestSignPayloadRefusesAmbiguousDocuments(t *testing.T) {
	for name, doc := range map[string]string{
		"float":         `{"a":1.5}`,
		"exponent":      `{"a":1e3}`,
		"duplicate":     `{"a":1,"a":2}`,
		"trailing":      `{"a":1} {"b":2}`,
		"not an object": `[1,2]`,
		"null":          `null`,
		"empty":         ``,
	} {
		if p, err := attest.SigningPayload([]byte(doc)); err == nil {
			t.Errorf("%s: payload %q for an ambiguous document", name, p)
		}
	}
}

func TestSignDocumentRawSetsProducer(t *testing.T) {
	key := newNodeKey(t)
	ev := goodEvidence() // producer coordinator-recorded, as the agent builds it before the coordinator accepts the frame
	raw, err := attest.SignDocumentRaw(marshal(t, ev), key)
	if err != nil {
		t.Fatal(err)
	}
	ev.Producer.Kind = "signed-remote"
	payload, _ := attest.SigningPayload(marshal(t, ev))
	if !ed25519.Verify(key.Public(), payload, raw) {
		t.Fatal("the raw signature is not over the signed-remote payload")
	}
	var obj map[string]json.RawMessage
	_ = json.Unmarshal(marshal(t, ev), &obj)
	delete(obj, "producer")
	noProducer, _ := json.Marshal(obj)
	if _, err := attest.SignDocumentRaw(noProducer, key); err == nil {
		t.Fatal("signed a document with no producer")
	}
}
