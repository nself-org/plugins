package crypto

import (
	"bytes"
	"context"
	"testing"
)

// Tenant isolation tests for BYOK envelope encryption.
//
// Purpose:   Verify that a record encrypted under tenant A's customer-managed
//            key (CMK) cannot be read using tenant B's CMK, and that each
//            tenant's wrapped DEK is bound to its own CMK reference.
// Inputs:    Two Envelope instances, each backed by a distinct mockKMS whose
//            maskByte models a different customer CMK.
// Outputs:   Pass when cross-tenant decryption fails or yields wrong bytes,
//            and the KMSKeyRef stamped on each bundle matches the tenant.
// Constraints: No real KMS calls; mockKMS (defined in envelope_test.go) is the
//            test double. AES-256-GCM authentication guarantees a wrong DEK
//            surfaces as a decryption error rather than silent plaintext.
// SPORT:     F04-PLUGIN-INVENTORY-PRO.md (byok row [7] Tests).

// tenantEnv builds an Envelope backed by a per-tenant CMK (distinct mask byte).
func tenantEnv(cmkMask byte) *Envelope {
	return NewEnvelope(&mockKMS{maskByte: cmkMask})
}

// TestTenantIsolationCrossDecryptFails verifies that ciphertext sealed for
// tenant A cannot be decrypted by tenant B's CMK. Because the DEK is wrapped by
// A's CMK, B's KMS unwraps to a different (wrong) DEK, and AES-256-GCM
// authentication rejects it — the spec requires an error, never wrong plaintext.
func TestTenantIsolationCrossDecryptFails(t *testing.T) {
	ctx := context.Background()
	tenantA := tenantEnv(0x11) // CMK for tenant A
	tenantB := tenantEnv(0x22) // CMK for tenant B

	secretA := []byte("tenant-A private record")

	bundle, err := tenantA.Encrypt(ctx, secretA, "arn:aws:kms:tenant-A/cmk")
	if err != nil {
		t.Fatalf("tenant A Encrypt: %v", err)
	}

	// Tenant B attempts to decrypt tenant A's bundle with B's CMK.
	got, err := tenantB.Decrypt(ctx, bundle)
	if err == nil {
		// If by astronomically unlikely chance no error, the plaintext MUST NOT match.
		if bytes.Equal(got, secretA) {
			t.Fatal("SECURITY: tenant B recovered tenant A plaintext — isolation broken")
		}
		t.Fatal("tenant B decrypt of tenant A bundle should fail authentication, got nil error")
	}
}

// TestTenantIsolationSelfDecryptSucceeds confirms the same CMK still round-trips,
// so the cross-tenant failure above is isolation and not a broken decrypt path.
func TestTenantIsolationSelfDecryptSucceeds(t *testing.T) {
	ctx := context.Background()
	tenantA := tenantEnv(0x11)

	secretA := []byte("tenant-A private record")
	bundle, err := tenantA.Encrypt(ctx, secretA, "arn:aws:kms:tenant-A/cmk")
	if err != nil {
		t.Fatalf("tenant A Encrypt: %v", err)
	}

	got, err := tenantA.Decrypt(ctx, bundle)
	if err != nil {
		t.Fatalf("tenant A self-decrypt should succeed: %v", err)
	}
	if !bytes.Equal(got, secretA) {
		t.Fatalf("tenant A self-decrypt: got %q, want %q", got, secretA)
	}
}

// TestTenantKeyRefStamped verifies each bundle records the CMK reference used to
// wrap its DEK, so audit and key-revocation logic can scope by tenant CMK.
func TestTenantKeyRefStamped(t *testing.T) {
	ctx := context.Background()
	tenantA := tenantEnv(0x11)
	tenantB := tenantEnv(0x22)

	refA := "arn:aws:kms:tenant-A/cmk"
	refB := "arn:aws:kms:tenant-B/cmk"

	bA, err := tenantA.Encrypt(ctx, []byte("a"), refA)
	if err != nil {
		t.Fatalf("tenant A Encrypt: %v", err)
	}
	bB, err := tenantB.Encrypt(ctx, []byte("b"), refB)
	if err != nil {
		t.Fatalf("tenant B Encrypt: %v", err)
	}

	if bA.KMSKeyRef != refA {
		t.Fatalf("tenant A KMSKeyRef = %q, want %q", bA.KMSKeyRef, refA)
	}
	if bB.KMSKeyRef != refB {
		t.Fatalf("tenant B KMSKeyRef = %q, want %q", bB.KMSKeyRef, refB)
	}
	if bA.KMSKeyRef == bB.KMSKeyRef {
		t.Fatal("two tenants share a KMSKeyRef — isolation metadata broken")
	}
}

// TestTenantWrappedDEKDiffers verifies that the same plaintext wrapped under two
// different tenant CMKs produces different wrapped-DEK bytes, so a leaked wrapped
// DEK from one tenant is useless against another.
func TestTenantWrappedDEKDiffers(t *testing.T) {
	ctx := context.Background()
	tenantA := tenantEnv(0x11)
	tenantB := tenantEnv(0x22)

	bA, err := tenantA.Encrypt(ctx, []byte("same"), "refA")
	if err != nil {
		t.Fatalf("tenant A Encrypt: %v", err)
	}
	bB, err := tenantB.Encrypt(ctx, []byte("same"), "refB")
	if err != nil {
		t.Fatalf("tenant B Encrypt: %v", err)
	}

	if bytes.Equal(bA.EncryptedDEK, bB.EncryptedDEK) {
		t.Fatal("wrapped DEKs identical across tenants — CMK wrapping not tenant-scoped")
	}
}
