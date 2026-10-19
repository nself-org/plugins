package verify

// Purpose: FuzzEnvelope, the fuzz target of the envelope reader and signature
// check (Epic CACHE D16): no input panics, and an input that verifies carries
// exactly the payload that was signed.

import (
	"crypto/ed25519"
	"encoding/json"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/signing"
)

func FuzzEnvelope(f *testing.F) {
	priv := ed25519.NewKeyFromSeed([]byte("0123456789abcdef0123456789abcdef"))
	pub := priv.Public().(ed25519.PublicKey)
	signer := signing.NewEd25519Signer(signing.PurposeCIRelease, priv)
	key := signing.Key{ID: signer.KeyID(), Purpose: signing.PurposeCIRelease, Public: pub}
	v, err := signing.NewVerifier(signing.PurposeCIRelease, []signing.Key{key}, nil)
	if err != nil {
		f.Fatal(err)
	}
	payload := []byte(`{"schema":"ci.release-manifest/v1"}`)
	env, err := signing.SignEnvelope(signer, "application/vnd.nself.ci.release-manifest.v1+json", payload)
	if err != nil {
		f.Fatal(err)
	}
	good, _ := json.Marshal(env)
	f.Add(good)
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"payloadType":"x","payload":"e30=","signatures":[]}`))
	f.Add([]byte(`{"payloadType":"x","payload":"e30=","signatures":[{"keyid":"a","sig":"b"},{"keyid":"a","sig":"b"}]}`))
	f.Add([]byte(`{"payloadType":"x","payloadType":"y"}`))
	f.Add([]byte("\xff\xfe"))
	f.Fuzz(func(t *testing.T, doc []byte) {
		e, err := parseEnvelope(doc)
		if err != nil {
			return
		}
		got, id, fail := verifyEnvelope(v, e, ReasonSignature)
		if fail != nil {
			if fail.Reason == "" || fail.ExitCode() == 0 {
				t.Fatalf("failure without a reason or exit: %+v", fail)
			}
			return
		}
		if id != signer.KeyID() || string(got) != string(payload) || e.PayloadType != "application/vnd.nself.ci.release-manifest.v1+json" {
			t.Fatalf("verified a document that is not the signed one: %q %q", id, got)
		}
	})
}
