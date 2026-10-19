package signing_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/signing"
)

func TestGenerateAndLoadRoundTrip(t *testing.T) {
	key, value, err := signing.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key.ID, "ci-release-") || len(key.ID) != len("ci-release-")+16 || len(key.Public) != ed25519.PublicKeySize {
		t.Fatalf("id %q", key.ID)
	}
	pemBytes, err := base64.StdEncoding.DecodeString(string(value))
	if err != nil || !strings.HasPrefix(string(pemBytes), "-----BEGIN PRIVATE KEY-----") {
		t.Fatal("the stored value is not base64 of a PEM")
	}
	res := &fakeRes{vals: map[string][]byte{signing.KeyRef: value}}
	loaded, err := signing.LoadKey(context.Background(), res, "att")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != key.ID || loaded.PublicB64() != key.PublicB64() || res.attempts[0] != "att" {
		t.Fatalf("loaded %s, generated %s", loaded.ID, key.ID)
	}
	sig, err := loaded.Signer.Sign([]byte("m"))
	if err != nil || !ed25519.Verify(key.Public, []byte("m"), sig) || loaded.Signer.KeyID() != key.ID {
		t.Fatal("the loaded key does not sign as the generated one")
	}
	loaded.Zero()
	if _, err := loaded.Signer.Sign([]byte("m")); err == nil {
		t.Fatal("a zeroed key still signed")
	}
	(*signing.Key)(nil).Zero() // nil-safe
}

func TestZeroOverwritesKeyBytes(t *testing.T) {
	key, _, _ := signing.Generate()
	pub := append([]byte(nil), key.Public...)
	key.Zero()
	if _, err := key.Signer.Sign([]byte("m")); !signing.IsReason(err, signing.ReasonSignerFailed) {
		t.Fatalf("a zeroed key signed: %v", err)
	}
	if string(pub) != string(key.Public) {
		t.Fatal("Zero changed the public key")
	}
}

func rsaValue(t *testing.T) []byte {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(k)
	return []byte(base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})))
}

func TestLoadKeyFailures(t *testing.T) {
	_, good, _ := signing.Generate()
	cases := []struct {
		name   string
		mod    func(r *fakeRes)
		reason string
		text   string
	}{
		{"missing", func(r *fakeRes) { delete(r.vals, signing.KeyRef) }, signing.ReasonKeyMissing, "key init"},
		{"not base64", func(r *fakeRes) { r.vals[signing.KeyRef] = []byte("!!!") }, signing.ReasonKeyInvalid, ""},
		{"not a pem", func(r *fakeRes) { r.vals[signing.KeyRef] = []byte(base64.StdEncoding.EncodeToString([]byte("hello"))) }, signing.ReasonKeyInvalid, ""},
		{"rsa key", func(r *fakeRes) { r.vals[signing.KeyRef] = rsaValue(t) }, signing.ReasonKeyInvalid, ""},
		{"stored id differs", func(r *fakeRes) { r.vals[signing.KeyIDRef] = []byte("ci-release-0123456789abcdef") }, signing.ReasonKeyIDMismatch, "ci-release-0123456789abcdef"},
		{"cli too old", func(r *fakeRes) { r.probeErr = &secrets.Error{Code: "E675", Message: "no resolve"} }, signing.ReasonCLITooOld, ""},
		{"resolver refuses", func(r *fakeRes) { r.resolveEr = &secrets.Error{Code: "E676", Cause: "E543", Message: "x"} }, signing.ReasonSignerFailed, "E676"},
		{"resolver error not coded", func(r *fakeRes) { r.resolveEr = errors.New("boom") }, signing.ReasonSignerFailed, ""},
	}
	for _, c := range cases {
		res := &fakeRes{vals: map[string][]byte{signing.KeyRef: good}}
		c.mod(res)
		_, err := signing.LoadKey(context.Background(), res, "att")
		if err == nil || reasonOf(t, err) != c.reason || !strings.Contains(err.Error(), c.text) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

func TestLoadKeyAcceptsMatchingStoredID(t *testing.T) {
	key, value, _ := signing.Generate()
	res := &fakeRes{vals: map[string][]byte{signing.KeyRef: value, signing.KeyIDRef: []byte(key.ID)}}
	k, err := signing.LoadKey(context.Background(), res, "att")
	if err != nil || k.ID != key.ID {
		t.Fatal(err)
	}
	res.vals[signing.KeyIDRef] = []byte(key.ID + "x")
	if _, err := signing.LoadKey(context.Background(), res, "att"); !signing.IsReason(err, signing.ReasonKeyIDMismatch) {
		t.Fatalf("a longer stored id matched: %v", err)
	}
}

func TestErrorsNeverCarryKeyBytes(t *testing.T) {
	_, value, _ := signing.Generate()
	secretText := string(value)
	for _, mod := range []func(r *fakeRes){
		func(r *fakeRes) { r.vals[signing.KeyRef] = append([]byte("A"), value...) },
		func(r *fakeRes) { r.vals[signing.KeyIDRef] = value },
	} {
		res := &fakeRes{vals: map[string][]byte{signing.KeyRef: value}}
		mod(res)
		if _, err := signing.LoadKey(context.Background(), res, "att"); err == nil || strings.Contains(err.Error(), secretText[:40]) {
			t.Fatalf("error %v", err)
		}
	}
}

func TestPublicReturnsOnlyThePublicHalf(t *testing.T) {
	key, value, _ := signing.Generate()
	res := &fakeRes{vals: map[string][]byte{signing.KeyRef: value}}
	info, err := signing.Public(context.Background(), res, signing.KeyAttempt)
	if err != nil || info.KeyID != key.ID || info.Public != key.PublicB64() || info.TrustLine() != key.ID+" "+key.PublicB64() || info.OldKeyID != "" {
		t.Fatalf("%+v %v", info, err)
	}
	delete(res.vals, signing.KeyRef)
	if _, err := signing.Public(context.Background(), res, signing.KeyAttempt); !signing.IsReason(err, signing.ReasonKeyMissing) {
		t.Fatal(err)
	}
}
