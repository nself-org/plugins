package api

// Integration tests that exercise the envelope-encryption end-to-end path
// from env-configured KEK ring through wrap → store-shape → unwrap.
//
// These tests intentionally do NOT spin up Postgres or HTTP. The handler-layer
// integration tests covering JWT auth, device ownership (V05-F4), and audit
// rejection live in handlers_test.go. THIS file targets the crypto pipeline
// that S02-T17/T18 must verify before the staging deploy in S02-T19.
//
// What is covered here:
//
//   1. KEK loaded purely from env → Wrap → Marshal → Unmarshal → Unwrap
//   2. KEK rotation: old envelope still readable, new write uses new version
//   3. Missing KEK version returns typed ErrKEKVersionNotFound (no panic)
//   4. Cross-KEK isolation: V1 envelope cannot be decrypted by V2-only ring

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/nself-vault/internal/crypto"
)

const (
	kek1Hex = "0101010101010101010101010101010101010101010101010101010101010101"
	kek2Hex = "0202020202020202020202020202020202020202020202020202020202020202"
)

// clearEnv strips every VAULT_KEK_* and DirEnv from the process env so that
// Load() in subsequent steps sees only what the test sets.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			continue
		}
		k := kv[:eq]
		if strings.HasPrefix(k, "VAULT_KEK_V") ||
			k == "VAULT_CURRENT_KEK_VERSION" ||
			k == "VAULT_KEK_DIR" {
			t.Setenv(k, "")
		}
	}
}

// fakeDEK is a stand-in for the 32-byte symmetric key that production code
// would generate per (tenant, alias). The exact bytes don't matter — what
// matters is byte-for-byte equality across the wrap → marshal → unmarshal →
// unwrap round trip.
var fakeDEK = bytes.Repeat([]byte{0xAA}, 32)

func TestIntegration_EnvelopeRoundTrip_EnvOnly(t *testing.T) {
	clearEnv(t)
	t.Setenv("VAULT_KEK_V1", kek1Hex)
	t.Setenv("VAULT_KEK_DIR", t.TempDir()) // isolate from any host /etc/nself-vault/keks

	kr, err := crypto.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if kr.Current() != 1 {
		t.Fatalf("Current = %d, want 1", kr.Current())
	}

	env, err := crypto.WrapDEK(kr, fakeDEK)
	if err != nil {
		t.Fatalf("WrapDEK: %v", err)
	}

	// Serialise as it would be stored in np_vault_keys.encrypted_dek.
	raw, err := env.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Read back from "DB" and decrypt.
	got, err := crypto.UnmarshalEnvelope(raw)
	if err != nil {
		t.Fatalf("UnmarshalEnvelope: %v", err)
	}
	plain, err := crypto.UnwrapDEK(kr, got)
	if err != nil {
		t.Fatalf("UnwrapDEK: %v", err)
	}
	if !bytes.Equal(plain, fakeDEK) {
		t.Fatal("DEK plaintext mismatch after round trip")
	}
}

func TestIntegration_RotationOldEnvelopesStillReadable(t *testing.T) {
	// Stage 1: V1 only, write an envelope, persist its marshaled bytes.
	clearEnv(t)
	t.Setenv("VAULT_KEK_V1", kek1Hex)
	t.Setenv("VAULT_KEK_DIR", t.TempDir())

	krV1, err := crypto.Load()
	if err != nil {
		t.Fatalf("Load V1: %v", err)
	}
	envV1, err := crypto.WrapDEK(krV1, fakeDEK)
	if err != nil {
		t.Fatalf("WrapDEK V1: %v", err)
	}
	if envV1.KEKVersion != 1 {
		t.Fatalf("envV1.KEKVersion = %d, want 1", envV1.KEKVersion)
	}
	stored, err := envV1.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Stage 2: rotation deploys V2 alongside V1. Highest-version-wins makes
	// V2 the new write key without an explicit selector. The stored V1
	// envelope must still decrypt.
	t.Setenv("VAULT_KEK_V2", kek2Hex)
	krRotated, err := crypto.Load()
	if err != nil {
		t.Fatalf("Load rotated: %v", err)
	}
	if krRotated.Current() != 2 {
		t.Fatalf("Current after rotation = %d, want 2", krRotated.Current())
	}

	// Decrypt old envelope using rotated ring (V1 KEK still present).
	loaded, err := crypto.UnmarshalEnvelope(stored)
	if err != nil {
		t.Fatalf("UnmarshalEnvelope: %v", err)
	}
	if loaded.KEKVersion != 1 {
		t.Fatalf("stored.KEKVersion = %d, want 1", loaded.KEKVersion)
	}
	plain, err := crypto.UnwrapDEK(krRotated, loaded)
	if err != nil {
		t.Fatalf("UnwrapDEK old envelope: %v", err)
	}
	if !bytes.Equal(plain, fakeDEK) {
		t.Fatal("old-envelope DEK mismatch after rotation")
	}

	// Stage 3: new envelope under rotated ring uses V2.
	envV2, err := crypto.WrapDEK(krRotated, fakeDEK)
	if err != nil {
		t.Fatalf("WrapDEK V2: %v", err)
	}
	if envV2.KEKVersion != 2 {
		t.Fatalf("envV2.KEKVersion = %d, want 2", envV2.KEKVersion)
	}
}

func TestIntegration_ReWrapMigratesOldEnvelopeToCurrentKEK(t *testing.T) {
	clearEnv(t)
	t.Setenv("VAULT_KEK_V1", kek1Hex)
	t.Setenv("VAULT_KEK_DIR", t.TempDir())

	krV1, err := crypto.Load()
	if err != nil {
		t.Fatalf("Load V1: %v", err)
	}
	envOld, err := crypto.WrapDEK(krV1, fakeDEK)
	if err != nil {
		t.Fatalf("WrapDEK: %v", err)
	}

	t.Setenv("VAULT_KEK_V2", kek2Hex)
	krRotated, err := crypto.Load()
	if err != nil {
		t.Fatalf("Load rotated: %v", err)
	}

	envNew, err := crypto.ReWrap(krRotated, envOld)
	if err != nil {
		t.Fatalf("ReWrap: %v", err)
	}
	if envNew.KEKVersion != 2 {
		t.Fatalf("ReWrap target version = %d, want 2", envNew.KEKVersion)
	}
	if bytes.Equal(envNew.Nonce, envOld.Nonce) {
		t.Fatal("ReWrap reused nonce")
	}
	plain, err := crypto.UnwrapDEK(krRotated, envNew)
	if err != nil {
		t.Fatalf("UnwrapDEK re-wrapped: %v", err)
	}
	if !bytes.Equal(plain, fakeDEK) {
		t.Fatal("ReWrap altered plaintext")
	}
}

func TestIntegration_MissingKEKVersionReturnsTypedError(t *testing.T) {
	// Write under a V2-only ring, then try to load with a V1-only ring.
	// Simulates a misconfigured rollback where the ops team removed V2 from
	// env before re-wrapping every envelope.
	clearEnv(t)
	t.Setenv("VAULT_KEK_V1", kek1Hex)
	t.Setenv("VAULT_KEK_V2", kek2Hex)
	t.Setenv("VAULT_KEK_DIR", t.TempDir())

	krBoth, err := crypto.Load()
	if err != nil {
		t.Fatalf("Load both: %v", err)
	}
	envV2, err := crypto.WrapDEK(krBoth, fakeDEK)
	if err != nil {
		t.Fatalf("WrapDEK: %v", err)
	}

	// Now simulate operator removing VAULT_KEK_V2 before re-wrap completed.
	t.Setenv("VAULT_KEK_V2", "")
	krV1Only, err := crypto.Load()
	if err != nil {
		t.Fatalf("Load V1-only: %v", err)
	}
	if krV1Only.Has(2) {
		t.Fatal("krV1Only unexpectedly has V2")
	}

	_, err = crypto.UnwrapDEK(krV1Only, envV2)
	if !errors.Is(err, crypto.ErrKEKVersionNotFound) {
		t.Fatalf("UnwrapDEK missing V2: want ErrKEKVersionNotFound, got %v", err)
	}
}

func TestIntegration_CrossKEKIsolation(t *testing.T) {
	// An envelope sealed with V1 must NOT decrypt against a fresh ring that
	// has a different value for V1 (e.g. test workstation vs. prod). This
	// guards against accidentally swapping KEK values across environments.
	clearEnv(t)
	t.Setenv("VAULT_KEK_V1", kek1Hex)
	t.Setenv("VAULT_KEK_DIR", t.TempDir())

	krEnvA, err := crypto.Load()
	if err != nil {
		t.Fatalf("Load A: %v", err)
	}
	env, err := crypto.WrapDEK(krEnvA, fakeDEK)
	if err != nil {
		t.Fatalf("WrapDEK: %v", err)
	}

	// Swap V1 to a DIFFERENT value (simulates wrong env).
	t.Setenv("VAULT_KEK_V1", kek2Hex)
	krEnvB, err := crypto.Load()
	if err != nil {
		t.Fatalf("Load B: %v", err)
	}
	if _, err := crypto.UnwrapDEK(krEnvB, env); err == nil {
		t.Fatal("envelope decrypted under wrong KEK material — isolation broken")
	}
}

func TestIntegration_CurrentVersionSelectorRespected(t *testing.T) {
	// V2 is configured but VAULT_CURRENT_KEK_VERSION pins writes to V1.
	// Used during the staging phase of a rotation, before promoting V2.
	clearEnv(t)
	t.Setenv("VAULT_KEK_V1", kek1Hex)
	t.Setenv("VAULT_KEK_V2", kek2Hex)
	t.Setenv("VAULT_CURRENT_KEK_VERSION", "1")
	t.Setenv("VAULT_KEK_DIR", t.TempDir())

	kr, err := crypto.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if kr.Current() != 1 {
		t.Fatalf("Current = %d, want 1 (pinned)", kr.Current())
	}
	env, err := crypto.WrapDEK(kr, fakeDEK)
	if err != nil {
		t.Fatalf("WrapDEK: %v", err)
	}
	if env.KEKVersion != 1 {
		t.Fatalf("envelope written with V%d, want V1", env.KEKVersion)
	}
	// V2 still readable for any pre-pinned envelopes.
	if !kr.Has(2) {
		t.Fatal("kr.Has(2) = false; V2 must remain available for decrypt")
	}
}
