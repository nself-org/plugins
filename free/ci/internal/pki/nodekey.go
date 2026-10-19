package pki

// Purpose: the pinned Ed25519 node key (Epic D5): pinned at enrollment (or at the first hello of an SSH
//          node), checked on every later hello, replaced only by `nodes rotate <node> --node-key`.
// Inputs:  node id, the node's public key as sent in hello.
// Outputs: the key id, errors with E660 reason node_key_changed.
// Constraints: the key id is the first 16 hex characters of SHA-256 of the raw public key (the same form
//          the agent prints); a pending pin holds the full SHA-256 digest (64 hex); a pin is a TOFU decision made at enrollment under an operator-confirmed
//          fingerprint, never silently re-pinned.

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"regexp"
)

var keyDigestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// NodeKeyDigest is the full SHA-256 of an Ed25519 node public key, 64 lower-case hex characters. A
// pending pin (rotate --node-key) binds this, not the 64-bit id.
func NodeKeyDigest(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:])
}

// NodeKeyID is the short id of an Ed25519 node public key (the first 16 hex characters of its digest).
func NodeKeyID(pub ed25519.PublicKey) string {
	return NodeKeyDigest(pub)[:16]
}

// pendingMatches reports whether pub is the key a pending pin names. A pin set by rotate holds the full
// digest; one written before the digest was required holds the 16-hex id and still matches by it.
func pendingMatches(pending string, pub ed25519.PublicKey) bool {
	if len(pub) != ed25519.PublicKeySize {
		return false
	}
	want := NodeKeyDigest(pub)
	if len(pending) == len(want) {
		return subtle.ConstantTimeCompare([]byte(pending), []byte(want)) == 1
	}
	return subtle.ConstantTimeCompare([]byte(pending), []byte(want[:len(pending)])) == 1
}

// PinNodeKey records the node key a node enrolled with.
func (c *CA) PinNodeKey(ctx context.Context, nodeID string, pub ed25519.PublicKey) error {
	if len(pub) != ed25519.PublicKeySize {
		return fail(ReasonBadCSR, "node key must be an Ed25519 public key")
	}
	return c.st.PKINodeKeyPut(ctx, nodeID, NodeKeyID(pub), base64.StdEncoding.EncodeToString(pub))
}

// CheckNodeKey compares a hello's node key with the pin. With no pin it pins (an SSH node's first
// hello). A pending pin (after rotate) accepts exactly the key id the operator named.
func (c *CA) CheckNodeKey(ctx context.Context, nodeID string, pub ed25519.PublicKey) error {
	keyID, pinned, ok, err := c.st.PKINodeKeyGet(ctx, nodeID)
	if err != nil {
		return err
	}
	got := base64.StdEncoding.EncodeToString(pub)
	switch {
	case !ok:
		return c.PinNodeKey(ctx, nodeID, pub)
	case pinned == "" && pendingMatches(keyID, pub):
		return c.PinNodeKey(ctx, nodeID, pub)
	case pinned != "" && subtle.ConstantTimeCompare([]byte(pinned), []byte(got)) == 1:
		return nil
	}
	return fail(ReasonNodeKeyChanged, "node %s presented a node key other than the pinned one", nodeID)
}

// CheckPinnedNodeKey is the read-only check a re-pairing node must pass: a pin must exist and match (a
// pending pin after rotate matches the named key id) and nothing is written; a node with no pin is
// refused. Pin with PinNodeKey once the enrollment is approved.
func (c *CA) CheckPinnedNodeKey(ctx context.Context, nodeID string, pub ed25519.PublicKey) error {
	keyID, pinned, ok, err := c.st.PKINodeKeyGet(ctx, nodeID)
	if err != nil {
		return err
	}
	if !ok {
		return fail(ReasonNodeKeyChanged, "node %s has no pinned node key; set one with `nodes rotate --node-key --key-id`", nodeID)
	}
	if len(pub) != ed25519.PublicKeySize || (pinned == "" && !pendingMatches(keyID, pub)) || (pinned != "" && subtle.ConstantTimeCompare([]byte(pinned), []byte(base64.StdEncoding.EncodeToString(pub))) != 1) {
		return fail(ReasonNodeKeyChanged, "node %s presented a node key other than the pinned one", nodeID)
	}
	return nil
}

// RotateNodeKey makes the key with this digest the only key the next hello of the node may present
// (`nodes rotate <node> --node-key --key-id <digest>`; the digest is what `join` printed).
func (c *CA) RotateNodeKey(ctx context.Context, actor, nodeID, digest string) error {
	if !keyDigestPattern.MatchString(digest) {
		return fail(ReasonNodeKeyChanged, "node key id must be the 64 lower-case hex characters of the SHA-256 that `join` prints")
	}
	return c.st.PKINodeKeyRotate(ctx, actor, nodeID, digest)
}
