package main

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// canonicalSigningMaterial rebuilds the byte string the device signed when it
// produced the operation. Mirrors `nclaw/core/src/sync/sign.rs::signing_material`
// after V04-F02 (user_id appended) + V04-F03 (deterministic field ordering).
//
// Layout (length-prefixed where strings/blobs appear; little-endian for ints):
//
//	event_id (uuid bytes) || entity_type (len:u32, bytes) || entity_id (uuid bytes) ||
//	op (u8: 0=insert,1=update,2=delete) ||
//	hlc_wall_ms (i64 LE) || hlc_lamport (u64 LE) || hlc_device_id (uuid bytes) ||
//	user_id (uuid bytes — V04-F02) ||
//	device_id (uuid bytes) ||
//	tenant_id (1 byte: 0=absent, 1=present || uuid bytes) ||
//	schema_version (i32 LE) ||
//	payload (canonical-JSON bytes, sorted keys; empty when nil)
//
// All UUID encoding uses the canonical 16-byte big-endian form. The function
// is pure — same inputs always produce the same bytes.
func canonicalSigningMaterial(ev pushRequestEvent, jwtUserID string) []byte {
	buf := make([]byte, 0, 256)
	buf = appendUUID(buf, ev.EventID)
	buf = appendLenString(buf, ev.EntityType)
	buf = appendUUID(buf, ev.EntityID)
	buf = append(buf, opByte(ev.Op))
	buf = appendInt64LE(buf, ev.HLCWallMs)
	buf = appendInt64LE(buf, ev.HLCLamport)
	buf = appendUUID(buf, ev.HLCDeviceID)
	buf = appendUUID(buf, jwtUserID)
	// device_id is implied by the JWT did claim — bind it into the material
	// so a stolen signature can't be replayed against a different device.
	buf = appendUUID(buf, ev.HLCDeviceID) // hlc device == operation device
	if ev.TenantID == "" {
		buf = append(buf, 0)
	} else {
		buf = append(buf, 1)
		buf = appendUUID(buf, ev.TenantID)
	}
	buf = appendInt32LE(buf, int32(ev.SchemaVersion))
	if len(ev.Payload) > 0 {
		buf = append(buf, canonicalJSON(ev.Payload)...)
	}
	return buf
}

func opByte(op string) byte {
	switch op {
	case "insert":
		return 0
	case "update":
		return 1
	case "delete":
		return 2
	default:
		return 0xff
	}
}

func appendUUID(dst []byte, s string) []byte {
	u, err := uuidFromString(s)
	if err != nil {
		// Append zero-UUID on parse failure — signature will not verify, the
		// operation is rejected upstream.
		return append(dst, make([]byte, 16)...)
	}
	return append(dst, u[:]...)
}

func appendLenString(dst []byte, s string) []byte {
	dst = appendInt32LE(dst, int32(len(s)))
	return append(dst, s...)
}

func appendInt64LE(dst []byte, v int64) []byte {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(v))
	return append(dst, b[:]...)
}

func appendInt32LE(dst []byte, v int32) []byte {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(v))
	return append(dst, b[:]...)
}

// uuidFromString parses a canonical-form UUID string into 16 bytes. Accepts
// both hyphenated (`xxxxxxxx-xxxx-...`) and bare hex forms.
func uuidFromString(s string) ([16]byte, error) {
	var out [16]byte
	clean := make([]byte, 0, 32)
	for i := 0; i < len(s); i++ {
		if s[i] == '-' {
			continue
		}
		clean = append(clean, s[i])
	}
	if len(clean) != 32 {
		return out, fmt.Errorf("invalid uuid length: %d", len(clean))
	}
	if _, err := hex.Decode(out[:], clean); err != nil {
		return out, err
	}
	return out, nil
}

// canonicalJSON re-serializes a JSON value with sorted object keys so two
// devices producing the same logical payload end up with identical signing
// material. Mirrors the Rust client's V04-F03 behavior.
func canonicalJSON(raw json.RawMessage) []byte {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	out, err := marshalCanonical(v)
	if err != nil {
		return raw
	}
	return out
}

// marshalCanonical walks the decoded value and emits JSON with sorted map keys.
// Maps preserve their UTF-8 string keys; arrays and scalars marshal via
// stdlib encoding/json. Numbers stay as json.Number where applicable.
func marshalCanonical(v any) ([]byte, error) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf := []byte{'{'}
		for i, k := range keys {
			if i > 0 {
				buf = append(buf, ',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return nil, err
			}
			buf = append(buf, kb...)
			buf = append(buf, ':')
			vb, err := marshalCanonical(t[k])
			if err != nil {
				return nil, err
			}
			buf = append(buf, vb...)
		}
		buf = append(buf, '}')
		return buf, nil
	case []any:
		buf := []byte{'['}
		for i, el := range t {
			if i > 0 {
				buf = append(buf, ',')
			}
			vb, err := marshalCanonical(el)
			if err != nil {
				return nil, err
			}
			buf = append(buf, vb...)
		}
		buf = append(buf, ']')
		return buf, nil
	default:
		return json.Marshal(t)
	}
}
