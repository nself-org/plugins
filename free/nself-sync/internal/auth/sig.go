// Package auth — Ed25519 event-envelope signature verification for nself-sync.
//
// Pairs with nclaw/core/src/sync/sign.rs on the client side. Signing material
// is byte-identical to the Rust implementation; any drift between the two is a
// security bug.
//
// V04-F02 fix (P102 S02 T06, 2026-05-14): signing material now binds the
// authenticated user_id as the first 16 bytes of the signed payload. A
// signature produced for user A on an envelope can no longer be presented
// to the server as if user B had authored it — the byte sequence under the
// signature differs, so the signature fails to verify.
//
// V04-F03 fix: the JSON payload portion uses RFC 8785 canonical JSON
// (see canonical.go) so signature bytes are deterministic across Go and Rust.
//
// Canonical signing material (concatenated, no separators):
//
//	user_id        16 bytes
//	event_id       16 bytes
//	entity_type    UTF-8 bytes (no length prefix)
//	entity_id      16 bytes
//	wall_ms        int64  little-endian (8 bytes)
//	lamport        uint64 little-endian (8 bytes)
//	dev_id (HLC)   16 bytes
//	op byte        1 byte  (0=insert, 1=update, 2=delete)
//	payload        RFC 8785 canonical JSON bytes (omitted if payload absent)
//
// Total minimum length when payload is empty: 16+16+0+16+8+8+16+1 = 81 bytes
// (entity_type contributes ≥1 byte in practice).
package auth

import (
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Op encodes an envelope operation type. Wire values MUST match
// crate::sync::lww::Op in the Rust client.
type Op uint8

const (
	OpInsert Op = 0
	OpUpdate Op = 1
	OpDelete Op = 2
)

// ParseOp converts the JSON string form ("insert"/"update"/"delete") to Op.
func ParseOp(s string) (Op, error) {
	switch s {
	case "insert":
		return OpInsert, nil
	case "update":
		return OpUpdate, nil
	case "delete":
		return OpDelete, nil
	default:
		return 0, fmt.Errorf("unknown op %q", s)
	}
}

// HLC mirrors the Rust crate::sync::hlc::Hlc struct on the wire.
type HLC struct {
	WallMS   int64     `json:"wall_ms"`
	Lamport  uint64    `json:"lamport"`
	DeviceID uuid.UUID `json:"device_id"`
}

// EventEnvelope mirrors crate::sync::lww::EventEnvelope. JSON tags match the
// over-the-wire field names used by the Rust client (serde defaults: snake_case).
type EventEnvelope struct {
	EventID       uuid.UUID  `json:"event_id"`
	EntityType    string     `json:"entity_type"`
	EntityID      uuid.UUID  `json:"entity_id"`
	Op            string     `json:"op"`
	Timestamp     HLC        `json:"timestamp"`
	UserID        uuid.UUID  `json:"user_id"`
	DeviceID      uuid.UUID  `json:"device_id"`
	TenantID      *uuid.UUID `json:"tenant_id,omitempty"`
	Payload       []byte     `json:"-"` // raw JSON bytes; populated by caller before Verify
	SchemaVersion uint32     `json:"schema_version"`
	Signature     []byte     `json:"-"` // raw 64-byte Ed25519 signature; populated by caller
}

// ErrInvalidSignature is returned when Ed25519 verification fails or input is malformed.
var ErrInvalidSignature = errors.New("invalid event signature")

// SigningMaterial returns the canonical byte sequence over which the Ed25519
// signature is computed. Output MUST be byte-identical to the Rust
// signing_material() function in nclaw/core/src/sync/sign.rs.
//
// `userID` is the asserted author identity. Pass the envelope's UserID for a
// straight verification; pass a different user_id to detect cross-user replay
// (the resulting bytes differ, so the signature will not verify).
//
// `payload` is the raw JSON payload bytes as they arrived on the wire. They are
// re-canonicalized via CanonicalJSON before inclusion so transport whitespace /
// key reordering does not break verification. Pass nil for envelopes with no
// payload (e.g., Delete events that carry only the entity ID).
func SigningMaterial(env *EventEnvelope, userID uuid.UUID, payload []byte) ([]byte, error) {
	op, err := ParseOp(env.Op)
	if err != nil {
		return nil, err
	}

	// Allocate up-front: 16+16+len(et)+16+8+8+16+1+payload
	buf := make([]byte, 0, 81+len(env.EntityType)+len(payload))

	// user_id (V04-F02)
	uid := userID
	buf = append(buf, uid[:]...)

	// event_id
	eid := env.EventID
	buf = append(buf, eid[:]...)

	// entity_type (no length prefix — Rust uses .as_bytes() directly)
	buf = append(buf, env.EntityType...)

	// entity_id
	entid := env.EntityID
	buf = append(buf, entid[:]...)

	// timestamp.wall_ms (i64 LE)
	var le8 [8]byte
	binary.LittleEndian.PutUint64(le8[:], uint64(env.Timestamp.WallMS))
	buf = append(buf, le8[:]...)

	// timestamp.lamport (u64 LE)
	binary.LittleEndian.PutUint64(le8[:], env.Timestamp.Lamport)
	buf = append(buf, le8[:]...)

	// timestamp.device_id
	hd := env.Timestamp.DeviceID
	buf = append(buf, hd[:]...)

	// op byte
	buf = append(buf, byte(op))

	// payload — canonicalize via RFC 8785 if present; match Rust canonical_json output.
	if len(payload) > 0 {
		canon, cerr := CanonicalJSON(payload)
		if cerr != nil {
			return nil, fmt.Errorf("canonicalize payload: %w", cerr)
		}
		buf = append(buf, canon...)
	}

	return buf, nil
}

// VerifyEvent checks the Ed25519 signature over `env` using `pubkey` (32 bytes),
// asserting that the envelope was authored by `userID`. Returns ErrInvalidSignature
// on any failure: malformed inputs, wrong key length, signature mismatch, or
// cross-user replay (where the signature was produced under a different user_id).
//
// Caller MUST also enforce env.UserID == userID at the auth layer; this function
// only checks cryptographic binding.
func VerifyEvent(env *EventEnvelope, userID uuid.UUID, payload []byte, pubkey []byte) error {
	if len(pubkey) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: pubkey wrong size (%d, want %d)",
			ErrInvalidSignature, len(pubkey), ed25519.PublicKeySize)
	}
	if len(env.Signature) != ed25519.SignatureSize {
		return fmt.Errorf("%w: signature wrong size (%d, want %d)",
			ErrInvalidSignature, len(env.Signature), ed25519.SignatureSize)
	}

	material, err := SigningMaterial(env, userID, payload)
	if err != nil {
		return fmt.Errorf("%w: build material: %v", ErrInvalidSignature, err)
	}

	if !ed25519.Verify(ed25519.PublicKey(pubkey), material, env.Signature) {
		return ErrInvalidSignature
	}
	return nil
}
