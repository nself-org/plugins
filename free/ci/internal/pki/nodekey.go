package pki

// Purpose: the pinned Ed25519 node key (Epic D5): pinned at enrollment (or at the first hello of an SSH
//          node), checked on every later hello, replaced only by `nodes rotate <node> --node-key`.
// Inputs:  node id, the node's public key as sent in hello.
// Outputs: the key id, errors with E660 reason node_key_changed.
// Constraints: the key id is the first 16 hex characters of SHA-256 of the raw public key (the same form
//          the agent prints); a pin is a TOFU decision made at enrollment under an operator-confirmed
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

var keyIDPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// NodeKeyID is the id of an Ed25519 node public key.
func NodeKeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:])[:16]
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
	case pinned == "" && len(pub) == ed25519.PublicKeySize && NodeKeyID(pub) == keyID:
		return c.PinNodeKey(ctx, nodeID, pub)
	case pinned != "" && subtle.ConstantTimeCompare([]byte(pinned), []byte(got)) == 1:
		return nil
	}
	return fail(ReasonNodeKeyChanged, "node %s presented a node key other than the pinned one", nodeID)
}

// CheckPinnedNodeKey is CheckNodeKey without the first-hello pin: a node with no pin is refused (a
// re-pairing node must already own its name).
func (c *CA) CheckPinnedNodeKey(ctx context.Context, nodeID string, pub ed25519.PublicKey) error {
	if _, _, ok, err := c.st.PKINodeKeyGet(ctx, nodeID); err != nil || !ok {
		if err == nil {
			err = fail(ReasonNodeKeyChanged, "node %s has no pinned node key; set one with `nodes rotate --node-key --key-id`", nodeID)
		}
		return err
	}
	return c.CheckNodeKey(ctx, nodeID, pub)
}

// RotateNodeKey makes keyID the only key the next hello of the node may present
// (`nodes rotate <node> --node-key --key-id <id>`; the id is what `join` printed).
func (c *CA) RotateNodeKey(ctx context.Context, actor, nodeID, keyID string) error {
	if !keyIDPattern.MatchString(keyID) {
		return fail(ReasonNodeKeyChanged, "node key id must be 16 lower-case hex characters")
	}
	return c.st.PKINodeKeyRotate(ctx, actor, nodeID, keyID)
}
