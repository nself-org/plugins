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
	"math/big"
	"regexp"
)

var keyIDPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// smallOrderY holds the y coordinates of the eight points of order dividing 8 on edwards25519 (0, 1, -1
// and the two pairs +-y8); every small-order point has one of them. An encoding is that y with a sign bit,
// and a y at or above p is a non-canonical spelling of y-p, so SmallOrderNodeKey reduces before it looks up.
var smallOrderY = func() []*big.Int {
	hexY := []string{
		"00",
		"01",
		"7a03ac9277fdc74ec6cc392cfa53202a0f67100d760b3cba4fd84d3d706a17c7", // y8
		"05fc536d880238b13933c6d305acdfd5f098eff289f4c345b027b2c28f95e826", // -y8
	}
	var out []*big.Int
	for _, h := range hexY {
		v, _ := new(big.Int).SetString(h, 16)
		out = append(out, v)
	}
	return append(out, new(big.Int).Sub(ed25519P, big.NewInt(1))) // -1
}()

var ed25519P = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(19))

// SmallOrderNodeKey reports whether pub encodes a small-order point. Ed25519 verification accepts such a
// key for crafted signatures on any message, so it is never a valid node key. The caller checks the length.
func SmallOrderNodeKey(pub []byte) bool {
	if len(pub) != ed25519.PublicKeySize {
		return false
	}
	le := make([]byte, len(pub))
	for i, b := range pub { // little endian to big endian, sign bit cleared
		le[len(pub)-1-i] = b
	}
	le[0] &= 0x7f
	y := new(big.Int).Mod(new(big.Int).SetBytes(le), ed25519P)
	for _, w := range smallOrderY {
		if y.Cmp(w) == 0 {
			return true
		}
	}
	return false
}

// NodeKeyID is the id of an Ed25519 node public key.
func NodeKeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:])[:16]
}

// PinNodeKey records the node key a node enrolled with.
func (c *CA) PinNodeKey(ctx context.Context, nodeID string, pub ed25519.PublicKey) error {
	if len(pub) != ed25519.PublicKeySize || SmallOrderNodeKey(pub) {
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
	if len(pub) != ed25519.PublicKeySize || (pinned == "" && NodeKeyID(pub) != keyID) || (pinned != "" && subtle.ConstantTimeCompare([]byte(pinned), []byte(base64.StdEncoding.EncodeToString(pub))) != 1) {
		return fail(ReasonNodeKeyChanged, "node %s presented a node key other than the pinned one", nodeID)
	}
	return nil
}

// RotateNodeKey makes keyID the only key the next hello of the node may present
// (`nodes rotate <node> --node-key --key-id <id>`; the id is what `join` printed).
func (c *CA) RotateNodeKey(ctx context.Context, actor, nodeID, keyID string) error {
	if !keyIDPattern.MatchString(keyID) {
		return fail(ReasonNodeKeyChanged, "node key id must be 16 lower-case hex characters")
	}
	return c.st.PKINodeKeyRotate(ctx, actor, nodeID, keyID)
}
