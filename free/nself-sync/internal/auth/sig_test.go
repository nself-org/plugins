package auth

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"testing"

	"github.com/google/uuid"
)

// goldenFixture matches nclaw/core/src/sync/sign.rs::signing_material_golden_fixture.
// If this byte sequence drifts from the Rust output, signatures will silently
// stop validating on real traffic — investigate immediately.
func goldenFixture(t *testing.T) (*EventEnvelope, []byte, uuid.UUID, []byte) {
	t.Helper()
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	eventID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	entityID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	deviceID := uuid.MustParse("44444444-4444-4444-4444-444444444444")

	env := &EventEnvelope{
		EventID:    eventID,
		EntityType: "Note",
		EntityID:   entityID,
		Op:         "insert",
		Timestamp: HLC{
			WallMS:   1_715_626_800_000,
			Lamport:  17,
			DeviceID: deviceID,
		},
		UserID:        userID,
		DeviceID:      deviceID,
		TenantID:      nil,
		SchemaVersion: 1,
	}
	// Already-canonical payload (sorted keys, no whitespace).
	payload := []byte(`{"k":"v"}`)

	// Expected signing material — must match the Rust golden_fixture test byte-for-byte.
	var expected []byte
	expected = append(expected, userID[:]...)   // 16
	expected = append(expected, eventID[:]...)  // 16
	expected = append(expected, []byte("Note")...) // 4
	expected = append(expected, entityID[:]...) // 16
	var le [8]byte
	binary.LittleEndian.PutUint64(le[:], uint64(1_715_626_800_000))
	expected = append(expected, le[:]...) // 8
	binary.LittleEndian.PutUint64(le[:], 17)
	expected = append(expected, le[:]...) // 8
	expected = append(expected, deviceID[:]...) // 16
	expected = append(expected, 0)              // op Insert
	expected = append(expected, []byte(`{"k":"v"}`)...)
	return env, payload, userID, expected
}

func TestSigningMaterial_GoldenFixture(t *testing.T) {
	env, payload, userID, expected := goldenFixture(t)

	got, err := SigningMaterial(env, userID, payload)
	if err != nil {
		t.Fatalf("SigningMaterial: %v", err)
	}
	if !bytes.Equal(got, expected) {
		t.Fatalf("byte mismatch with Rust golden fixture\n want: %x\n got:  %x", expected, got)
	}
	if len(got) != 94 {
		t.Fatalf("golden material length = %d, want 94", len(got))
	}
}

func TestSigningMaterial_NonCanonicalPayloadIsCanonicalized(t *testing.T) {
	env, _, userID, expected := goldenFixture(t)

	// Spaced + reordered payload — should canonicalize to {"k":"v"}.
	noisy := []byte(`{ "k" : "v" }`)
	got, err := SigningMaterial(env, userID, noisy)
	if err != nil {
		t.Fatalf("SigningMaterial: %v", err)
	}
	if !bytes.Equal(got, expected) {
		t.Fatalf("non-canonical payload not normalized\n want: %x\n got:  %x", expected, got)
	}
}

func TestSigningMaterial_DifferentUserIDProducesDifferentBytes(t *testing.T) {
	env, payload, userIDA, _ := goldenFixture(t)
	userIDB := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")

	matA, err := SigningMaterial(env, userIDA, payload)
	if err != nil {
		t.Fatalf("A: %v", err)
	}
	matB, err := SigningMaterial(env, userIDB, payload)
	if err != nil {
		t.Fatalf("B: %v", err)
	}
	if bytes.Equal(matA, matB) {
		t.Fatal("V04-F02 defense broken: same signing material for two different user_ids")
	}
}

// TestVerifyEvent_RejectsCrossUserReplay is the core V04-F02 regression test.
// A signature legitimately produced for user A on an envelope MUST NOT verify
// when the server is told to verify under user B's identity, even if the
// envelope's UserID field has been swapped to B.
func TestVerifyEvent_RejectsCrossUserReplay(t *testing.T) {
	env, payload, userIDA, _ := goldenFixture(t)
	userIDB := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")

	// Generate a real Ed25519 keypair for user A.
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	// Sign under user A.
	matA, err := SigningMaterial(env, userIDA, payload)
	if err != nil {
		t.Fatalf("material A: %v", err)
	}
	sig := ed25519.Sign(priv, matA)
	env.Signature = sig

	// Verifies under A.
	if err := VerifyEvent(env, userIDA, payload, pub); err != nil {
		t.Fatalf("verify under A should succeed, got %v", err)
	}

	// Replay attempt: server asked to verify under B with the same signature.
	if err := VerifyEvent(env, userIDB, payload, pub); err == nil {
		t.Fatal("V04-F02 defense broken: signature for user A verified under user B")
	}
}

func TestVerifyEvent_RejectsTamperedPayload(t *testing.T) {
	env, payload, userID, _ := goldenFixture(t)

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	mat, err := SigningMaterial(env, userID, payload)
	if err != nil {
		t.Fatalf("material: %v", err)
	}
	env.Signature = ed25519.Sign(priv, mat)

	tampered := []byte(`{"k":"v2"}`)
	if err := VerifyEvent(env, userID, tampered, pub); err == nil {
		t.Fatal("tampered payload verified — should have failed")
	}
}

func TestVerifyEvent_RejectsWrongKeySize(t *testing.T) {
	env, payload, userID, _ := goldenFixture(t)
	env.Signature = make([]byte, ed25519.SignatureSize)
	err := VerifyEvent(env, userID, payload, []byte{1, 2, 3})
	if err == nil {
		t.Fatal("short pubkey accepted")
	}
}

func TestVerifyEvent_RejectsWrongSignatureSize(t *testing.T) {
	env, payload, userID, _ := goldenFixture(t)
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	env.Signature = []byte{0xaa, 0xbb}
	if err := VerifyEvent(env, userID, payload, pub); err == nil {
		t.Fatal("short signature accepted")
	}
}

func TestParseOp(t *testing.T) {
	cases := []struct {
		in   string
		want Op
		err  bool
	}{
		{"insert", OpInsert, false},
		{"update", OpUpdate, false},
		{"delete", OpDelete, false},
		{"INSERT", 0, true},
		{"", 0, true},
		{"deleted", 0, true},
	}
	for _, tc := range cases {
		got, err := ParseOp(tc.in)
		if tc.err {
			if err == nil {
				t.Errorf("ParseOp(%q) want error, got %d", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseOp(%q) unexpected error %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("ParseOp(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
