package attest

// Purpose: the signed audit checkpoint (Epic D13/D20, X6): the TEAM coordinator signs the head of the
// hash chain with its own ci-audit key, so a writer of the database who recomputes the whole chain is
// still caught by a checkpoint held elsewhere.
// Inputs:  the chain head, the row count and sequence, a signing.Signer of purpose ci-audit; for verify
//          the checkpoint document and a public-key fingerprint pinned off-host.
// Outputs: Checkpoint (JSON), Fingerprint, VerifyCheckpoint, LoadOrCreateCheckpointKey.
// Constraints: Ed25519 through sdk/go/signing only; the key lives in a 0600 file under the coordinator's
//          data dir and is created on first export; VerifyCheckpoint trusts only the key whose SHA-256
//          fingerprint equals the pinned one, never the key in the data dir; comparing the head with the
//          live chain is the caller's step (it needs the store).

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/signing"
	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// CheckpointPrefix is the domain separation label of a checkpoint signature.
const CheckpointPrefix = "nself-ci-audit-checkpoint/v1\n"

// CheckpointKeyFile is the signing key under the coordinator's data dir.
const CheckpointKeyFile = "audit-checkpoint.key"

// Checkpoint is a signed statement of the audit chain head.
type Checkpoint struct {
	Version   int    `json:"version"`
	Head      string `json:"head"` // lower-case hex of the head of row Seq
	Seq       int64  `json:"seq"`
	Rows      int64  `json:"rows"`
	TS        int64  `json:"ts"` // unix nanoseconds
	KeyID     string `json:"key_id"`
	Public    string `json:"public"`    // standard base64 of the raw public key
	Signature string `json:"signature"` // signing.EncodeSig of the signature
}

func (c Checkpoint) payload() ([]byte, error) {
	doc, err := trust.Canonical(map[string]any{"version": c.Version, "head": c.Head, "seq": c.Seq, "rows": c.Rows, "ts": c.TS, "key_id": c.KeyID})
	return append([]byte(CheckpointPrefix), doc...), err
}

// Fingerprint is the lower-case hex SHA-256 of the raw public key: what an operator pins off-host.
func Fingerprint(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:])
}

// NewCheckpoint signs head (the head of audit row seq among rows rows) at ts.
func NewCheckpoint(s signing.Signer, pub ed25519.PublicKey, head []byte, seq, rows, ts int64) (Checkpoint, error) {
	if s == nil || len(head) == 0 || seq < 1 || rows < 1 {
		return Checkpoint{}, errors.New("attest: a checkpoint needs a signer and a non-empty chain")
	}
	c := Checkpoint{Version: 1, Head: hex.EncodeToString(head), Seq: seq, Rows: rows, TS: ts, KeyID: s.KeyID(), Public: base64.StdEncoding.EncodeToString(pub)}
	if c.KeyID != signing.KeyID(signing.PurposeCIAudit, pub) {
		return Checkpoint{}, errors.New("attest: the signer is not the ci-audit key of the public key")
	}
	msg, err := c.payload()
	if err != nil {
		return Checkpoint{}, err
	}
	sig, err := s.Sign(msg)
	if err != nil {
		return Checkpoint{}, err
	}
	c.Signature = signing.EncodeSig(sig)
	return c, nil
}

// VerifyCheckpoint checks c against a pinned fingerprint (64 hex characters, optionally "sha256:"
// prefixed): the embedded key must have that fingerprint and the signature must verify under it.
func VerifyCheckpoint(c Checkpoint, pinned string) error {
	pinned = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(pinned), "sha256:"))
	if len(pinned) != 64 {
		return errors.New("attest: the pinned fingerprint is 64 hex characters")
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(c.Public)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return errors.New("attest: the checkpoint carries no valid public key")
	}
	pub := ed25519.PublicKey(raw)
	if subtle.ConstantTimeCompare([]byte(Fingerprint(pub)), []byte(pinned)) != 1 {
		return errors.New("attest: the checkpoint key is not the pinned key")
	}
	if c.Version != 1 || c.KeyID != signing.KeyID(signing.PurposeCIAudit, pub) {
		return errors.New("attest: unsupported checkpoint or key id")
	}
	v, err := signing.NewVerifier(signing.PurposeCIAudit, []signing.Key{{ID: c.KeyID, Purpose: signing.PurposeCIAudit, Public: pub}}, nil)
	if err != nil {
		return err
	}
	sig, err := signing.DecodeSig(c.Signature)
	if err != nil {
		return err
	}
	msg, err := c.payload()
	if err != nil {
		return err
	}
	return v.Verify(msg, signing.Signature{KeyID: c.KeyID, Sig: sig})
}

// LoadOrCreateCheckpointKey returns the checkpoint signer under dir, creating the key (0600, the
// directory 0700) on first use. The file is never replaced.
func LoadOrCreateCheckpointKey(dir string) (signing.Signer, ed25519.PublicKey, error) {
	if err := nodekey.EnsureDir(dir); err != nil {
		return nil, nil, err
	}
	path := filepath.Join(dir, CheckpointKeyFile)
	for range 2 {
		if err := nodekey.CheckPrivateFile(path); err == nil {
			data, rerr := os.ReadFile(path)
			if rerr != nil {
				return nil, nil, rerr
			}
			priv, perr := signing.ParsePKCS8PEM(data)
			if perr != nil {
				return nil, nil, fmt.Errorf("%s: %w", path, perr)
			}
			return signing.NewEd25519Signer(signing.PurposeCIAudit, priv), priv.Public().(ed25519.PublicKey), nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, nil, err
		}
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, nil, err
		}
		der, err := x509.MarshalPKCS8PrivateKey(priv)
		if err != nil {
			return nil, nil, err
		}
		if err = nodekey.WriteExclusive(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})); err != nil && !errors.Is(err, fs.ErrExist) {
			return nil, nil, err
		}
	}
	return nil, nil, fmt.Errorf("checkpoint key %s could not be created", path)
}
