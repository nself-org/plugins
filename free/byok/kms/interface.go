// Package kms defines the KMSProvider interface and key metadata types for
// BYOK envelope encryption. Each KMS backend (AWS, GCP, Vault) implements
// this interface.
package kms

import "context"

// KeyMetadata describes a customer-managed key.
type KeyMetadata struct {
	KeyRef    string // provider-specific key identifier
	Provider  string // "aws" | "gcp" | "vault"
	Algorithm string // e.g. "SYMMETRIC_DEFAULT", "AES_256"
	State     string // "ENABLED" | "DISABLED" | "PENDING_DELETION"
	CreatedAt string // RFC3339
}

// KMSProvider is the single interface every KMS backend must implement.
//
// WrapKey encrypts a plaintext DEK using the customer's CMK identified by
// keyRef. It returns the opaque wrapped (ciphertext) DEK bytes.
//
// UnwrapKey decrypts a wrapped DEK. It returns the plaintext DEK bytes.
// If the CMK has been revoked or suspended, the provider returns a
// non-nil error — callers MUST NOT fall back to a cached plaintext DEK
// in that case (see cache/dek_cache.go).
//
// DescribeKey fetches metadata for a given key reference.
type KMSProvider interface {
	WrapKey(ctx context.Context, plainDEK []byte, keyRef string) ([]byte, error)
	UnwrapKey(ctx context.Context, wrappedDEK []byte, keyRef string) ([]byte, error)
	DescribeKey(ctx context.Context, keyRef string) (KeyMetadata, error)
}

// ProviderName constants match the np_kms_configs.provider CHECK constraint.
const (
	ProviderAWS   = "aws"
	ProviderGCP   = "gcp"
	ProviderVault = "vault"
)
