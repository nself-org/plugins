package evidence_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/evidence"
)

func wire14Signer(t *testing.T, p signing.Purpose) signing.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return signing.NewEd25519Signer(p, priv)
}

func TestWire14SignLocalRefusesWhatItCannotSign(t *testing.T) {
	if _, err := evidence.SignLocal([]byte(`{"schema":"ci.evidence/v1"}`), nil); err == nil {
		t.Error("signed without a signer")
	}
	if _, err := evidence.SignLocal([]byte(`[1]`), wire14Signer(t, signing.PurposeCIRelease)); err == nil {
		t.Error("signed a document that is not an object")
	}
	if _, err := evidence.SignLocal(nil, wire14Signer(t, signing.PurposeCIRelease)); err == nil {
		t.Error("signed an empty document")
	}
}

func TestWire14EnvelopeWithoutTrustIsRefused(t *testing.T) {
	t.Setenv("NSELF_CI_HOME", t.TempDir()) // no trust files
	env, err := evidence.SignLocal([]byte(`{"schema":"ci.evidence/v1"}`), wire14Signer(t, signing.PurposeCIRelease))
	if err != nil {
		t.Fatal(err)
	}
	var probe map[string]any
	if err := json.Unmarshal(env, &probe); err != nil || probe["payloadType"] != evidence.LocalPayloadType {
		t.Fatalf("not an envelope: %v %v", err, probe)
	}
	v, err := evidence.VerifyDocument(env, "", nil, nil)
	var ee *evidence.Error
	if !errors.As(err, &ee) || ee.Exit != evidence.ExitInvalid || v.Exit != evidence.ExitInvalid {
		t.Fatalf("an envelope with no trusted key was not refused as invalid: %v %+v", err, v)
	}
}
