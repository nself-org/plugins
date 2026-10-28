package signing

// Purpose: the release key inside the signer (Epic CACHE D11/D12): resolve it
// by reference through the secret resolver, parse it, derive its key id,
// match a stored id, generate a new one, and zero everything afterwards.
// Inputs:  a secrets.ValueResolver and an attempt id.
// Outputs: *Key (signer, derived id, public key) or an E717 refusal.
// Constraints: the key is `release/NSELF_CI_RELEASE_SIGNING_KEY` (base64 of a
// PKCS#8 PEM, the P7-PROD-59 format), purpose ci-release, id
// ci-release-<16 hex>; a stored `..._KEY_ID` must equal the derived id; no
// error text carries key bytes; Zero overwrites the private key and every
// intermediate buffer; the key never leaves this process (no argv, no stdout,
// no file, no environment of a child).

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"regexp"

	"github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/secrets"
)

// Refs of the release key and its optional stored id.
const (
	KeyRef   = "release/NSELF_CI_RELEASE_SIGNING_KEY"
	KeyIDRef = "release/NSELF_CI_RELEASE_SIGNING_KEY_ID"
	// KeyName is the secret name `config secrets set --env release` takes.
	KeyName = "NSELF_CI_RELEASE_SIGNING_KEY"
)

var keyIDRe = regexp.MustCompile(`^ci-release-[0-9a-f]{16}$`)

// notFoundCause is the CLI's code for a ref that is not in the store.
const notFoundCause = "E541"

// Key is a loaded release key. Zero it when done.
type Key struct {
	Signer signing.Signer
	ID     string
	Public ed25519.PublicKey
	priv   ed25519.PrivateKey
	zeroed bool
}

// Zero overwrites the private key bytes.
func (k *Key) Zero() {
	if k == nil {
		return
	}
	clear(k.priv)
	k.zeroed = true
}

// PublicB64 is the base64 public key of a trust line.
func (k *Key) PublicB64() string { return base64.StdEncoding.EncodeToString(k.Public) }

// ownSigner signs with the key held in Key.priv itself (the sdk signer keeps
// a private copy Zero could not overwrite).
type ownSigner struct {
	id  string
	key *Key
}

func (s *ownSigner) KeyID() string { return s.id }

func (s *ownSigner) Sign(msg []byte) ([]byte, error) {
	if s.key.zeroed || len(s.key.priv) != ed25519.PrivateKeySize {
		return nil, Failf(ReasonSignerFailed, "the key was already zeroed")
	}
	return ed25519.Sign(s.key.priv, msg), nil
}

func newKey(priv ed25519.PrivateKey) *Key {
	pub, _ := priv.Public().(ed25519.PublicKey)
	id := signing.KeyID(signing.PurposeCIRelease, pub)
	k := &Key{ID: id, Public: pub, priv: priv}
	k.Signer = &ownSigner{id: id, key: k}
	return k
}

// mapResolve turns a resolver error into an E717 refusal.
func mapResolve(err error) error {
	var se *secrets.Error
	if !errors.As(err, &se) {
		return Failf(ReasonSignerFailed, "the secret resolver failed")
	}
	if se.Code == "E675" {
		return Failf(ReasonCLITooOld, "this nself cli has no `config secrets resolve`: upgrade it")
	}
	if se.Code == "E676" && se.Cause == notFoundCause {
		return Failf(ReasonKeyMissing, "the release key is not set: run `nself ci artifacts key init`")
	}
	return Failf(ReasonSignerFailed, "the release key could not be resolved (%s)", echo(se.Code))
}

// resolveOne resolves one ref and returns its value bytes (caller zeroes them).
func resolveOne(ctx context.Context, res secrets.ValueResolver, attempt, raw string) ([]byte, error) {
	ref, err := secrets.ParseSigner(raw)
	if err != nil {
		return nil, Failf(ReasonSignerFailed, "invalid signer secret reference")
	}
	if err := res.Probe(ctx); err != nil {
		return nil, mapResolve(err)
	}
	vals, err := res.Resolve(ctx, attempt, []secrets.Ref{ref})
	if err != nil {
		return nil, mapResolve(err)
	}
	if len(vals) != 1 {
		for _, v := range vals {
			clear(v.Bytes)
		}
		return nil, Failf(ReasonSignerFailed, "the secret resolver answered the wrong number of values")
	}
	return vals[0].Bytes, nil
}

// LoadKey resolves and parses the release key for an attempt. A stored key id
// that differs from the derived one is key_id_mismatch.
func LoadKey(ctx context.Context, res secrets.ValueResolver, attempt string) (*Key, error) {
	return loadKey(ctx, res, attempt, true)
}

// loadKey is LoadKey with the stored-id check optional: key init and key
// rotate replace both values, so a stale id must not stop them.
func loadKey(ctx context.Context, res secrets.ValueResolver, attempt string, checkID bool) (*Key, error) {
	raw, err := resolveOne(ctx, res, attempt, KeyRef)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	pemBytes := make([]byte, base64.StdEncoding.DecodedLen(len(raw)))
	defer clear(pemBytes)
	n, derr := base64.StdEncoding.Strict().Decode(pemBytes, raw)
	if derr != nil {
		return nil, Failf(ReasonKeyInvalid, "the release key is not base64 of a PKCS#8 PEM")
	}
	priv, perr := signing.ParsePKCS8PEM(pemBytes[:n])
	if perr != nil {
		return nil, Failf(ReasonKeyInvalid, "the release key is not an Ed25519 PKCS#8 PEM")
	}
	key := newKey(priv)
	if checkID {
		if err := checkStoredID(ctx, res, attempt, key); err != nil {
			key.Zero()
			return nil, err
		}
	}
	return key, nil
}

// checkStoredID compares a stored NSELF_CI_RELEASE_SIGNING_KEY_ID, when there is one.
func checkStoredID(ctx context.Context, res secrets.ValueResolver, attempt string, key *Key) error {
	raw, err := resolveOne(ctx, res, attempt, KeyIDRef)
	if IsReason(err, ReasonKeyMissing) {
		return nil
	}
	if err != nil {
		return err
	}
	defer clear(raw)
	if string(raw) != key.ID {
		// a stored id of another shape is never echoed: a key pasted there by mistake must not reach a log
		shown := "malformed"
		if keyIDRe.Match(raw) {
			shown = string(raw)
		}
		return Failf(ReasonKeyIDMismatch, "the stored key id (%s) is not the id of the stored key (%s)", shown, key.ID)
	}
	return nil
}

// Generate makes a new key and returns it with the secret value to store
// (base64 of the PKCS#8 PEM). The caller zeroes both.
func Generate() (*Key, []byte, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, Failf(ReasonSignerFailed, "no key could be generated")
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		clear(priv)
		return nil, nil, Failf(ReasonSignerFailed, "no key could be encoded")
	}
	defer clear(der)
	block := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	defer clear(block)
	value := make([]byte, base64.StdEncoding.EncodedLen(len(block)))
	base64.StdEncoding.Encode(value, block)
	return newKey(priv), value, nil
}
