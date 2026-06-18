package internal

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"testing"

	"github.com/cloudflare/circl/kem/mlkem/mlkem1024"
)

// Pure-logic tests that do not require a live database. Atomic one-time-prekey
// consumption is enforced by the SQL `UPDATE ... WHERE id = (SELECT ... FOR
// UPDATE SKIP LOCKED LIMIT 1) RETURNING` statement in GetPreKeyBundle; that
// invariant is documented and exercised in integration, not unit, tests.

func TestLoadConfigDefaults(t *testing.T) {
	os.Unsetenv("E2EE_PLUGIN_PORT")
	os.Unsetenv("E2EE_PLUGIN_HOST")
	os.Unsetenv("E2EE_MAX_ONE_TIME_PREKEYS")
	cfg := LoadConfig()
	if cfg.Port != "3055" {
		t.Fatalf("default port: want 3055 got %s", cfg.Port)
	}
	if cfg.Host != "0.0.0.0" {
		t.Fatalf("default host: want 0.0.0.0 got %s", cfg.Host)
	}
	if cfg.MaxOneTimePreKeys != 100 || cfg.MaxKyberPreKeys != 100 {
		t.Fatalf("default prekey caps want 100/100 got %d/%d",
			cfg.MaxOneTimePreKeys, cfg.MaxKyberPreKeys)
	}
}

func TestLoadConfigOverrides(t *testing.T) {
	t.Setenv("E2EE_PLUGIN_PORT", "4099")
	t.Setenv("E2EE_MAX_ONE_TIME_PREKEYS", "50")
	t.Setenv("E2EE_MAX_KYBER_PREKEYS", "0") // invalid -> falls back to default
	cfg := LoadConfig()
	if cfg.Port != "4099" {
		t.Fatalf("port override failed: %s", cfg.Port)
	}
	if cfg.MaxOneTimePreKeys != 50 {
		t.Fatalf("otpk cap override failed: %d", cfg.MaxOneTimePreKeys)
	}
	if cfg.MaxKyberPreKeys != 100 {
		t.Fatalf("invalid kyber cap should fall back to 100, got %d", cfg.MaxKyberPreKeys)
	}
}

func TestDecodeB64(t *testing.T) {
	if _, err := decodeB64(""); err == nil {
		t.Fatal("empty string must error")
	}
	if _, err := decodeB64("!!!notb64!!!"); err == nil {
		t.Fatal("invalid base64 must error")
	}
	want := []byte("hello")
	got, err := decodeB64(base64.StdEncoding.EncodeToString(want))
	if err != nil {
		t.Fatalf("valid base64 errored: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("round-trip mismatch: %q", got)
	}
}

func TestNullableAndOrUnknown(t *testing.T) {
	if nullable("") != nil {
		t.Fatal("empty string should map to nil")
	}
	if nullable("x") != "x" {
		t.Fatal("non-empty should pass through")
	}
	if orUnknown("") != "unknown" {
		t.Fatal("empty should become 'unknown'")
	}
	if orUnknown("alice") != "alice" {
		t.Fatal("non-empty should pass through")
	}
}

// TestWireFormat_RoundTrip simulates the exact base64 wire encoding a client
// sends for a Kyber prekey upload, then runs the server-side verification path.
// This is the end-to-end KAT for the upload-verify flow.
func TestWireFormat_RoundTrip(t *testing.T) {
	idPub, idPriv, _ := ed25519.GenerateKey(rand.Reader)
	kPub, _, err := mlkem1024.Scheme().GenerateKeyPair()
	if err != nil {
		t.Fatalf("kyber keygen: %v", err)
	}
	kBytes, _ := kPub.MarshalBinary()
	sig := ed25519.Sign(idPriv, kBytes)

	// Encode as the client would.
	kB64 := base64.StdEncoding.EncodeToString(kBytes)
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	// Decode + verify as the server handler would.
	decodedK, err := decodeB64(kB64)
	if err != nil {
		t.Fatalf("decode kyber: %v", err)
	}
	decodedSig, err := decodeB64(sigB64)
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	if err := VerifyKyberPreKey(idPub, decodedK, decodedSig); err != nil {
		t.Fatalf("wire round-trip verification failed: %v", err)
	}
}
