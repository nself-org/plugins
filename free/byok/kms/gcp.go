package kms

import (
	"context"
	"encoding/base64"
	"fmt"

	"google.golang.org/api/cloudkms/v1"
	"google.golang.org/api/option"
)

// GCPKMSProvider wraps Google Cloud KMS for envelope encryption.
// Key references use the full resource path:
//
//	projects/{project}/locations/{location}/keyRings/{ring}/cryptoKeys/{key}
type GCPKMSProvider struct {
	svc *cloudkms.ProjectsLocationsKeyRingsCryptoKeysService
}

// NewGCPKMSProvider constructs a GCPKMSProvider using Application Default
// Credentials (ADC). Pass credentialsJSON as non-nil to use service account
// JSON directly instead of ADC.
func NewGCPKMSProvider(ctx context.Context, credentialsJSON []byte) (*GCPKMSProvider, error) {
	opts := []option.ClientOption{}
	if len(credentialsJSON) > 0 {
		opts = append(opts, option.WithCredentialsJSON(credentialsJSON))
	}
	svc, err := cloudkms.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("gcp kms: new service: %w", err)
	}
	return &GCPKMSProvider{
		svc: svc.Projects.Locations.KeyRings.CryptoKeys,
	}, nil
}

// WrapKey encrypts plainDEK under the GCP KMS symmetric key at keyRef using
// the raw encrypt API. The Google API discovery client transports binary data
// as base64-encoded strings on the wire, so plaintext is base64-encoded before
// the request and the returned ciphertext (already base64) is decoded back to
// raw bytes.
func (p *GCPKMSProvider) WrapKey(ctx context.Context, plainDEK []byte, keyRef string) ([]byte, error) {
	req := &cloudkms.EncryptRequest{
		Plaintext: base64.StdEncoding.EncodeToString(plainDEK),
	}
	resp, err := p.svc.Encrypt(keyRef, req).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("gcp kms: encrypt DEK: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(resp.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("gcp kms: decode ciphertext: %w", err)
	}
	return ciphertext, nil
}

// UnwrapKey decrypts wrappedDEK using GCP KMS. If the key is disabled or
// destroyed, the API returns an error — no fallback is attempted. Wire format
// is base64 in both directions; raw bytes are encoded for the request and the
// returned base64 plaintext is decoded back before return.
func (p *GCPKMSProvider) UnwrapKey(ctx context.Context, wrappedDEK []byte, keyRef string) ([]byte, error) {
	req := &cloudkms.DecryptRequest{
		Ciphertext: base64.StdEncoding.EncodeToString(wrappedDEK),
	}
	resp, err := p.svc.Decrypt(keyRef, req).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("gcp kms: decrypt DEK: %w", err)
	}
	plaintext, err := base64.StdEncoding.DecodeString(resp.Plaintext)
	if err != nil {
		return nil, fmt.Errorf("gcp kms: decode plaintext: %w", err)
	}
	return plaintext, nil
}

// DescribeKey fetches key version metadata from Cloud KMS.
func (p *GCPKMSProvider) DescribeKey(ctx context.Context, keyRef string) (KeyMetadata, error) {
	key, err := p.svc.Get(keyRef).Context(ctx).Do()
	if err != nil {
		return KeyMetadata{}, fmt.Errorf("gcp kms: describe key: %w", err)
	}
	return KeyMetadata{
		KeyRef:    keyRef,
		Provider:  ProviderGCP,
		Algorithm: key.Purpose,
		State:     "ENABLED",
		CreatedAt: key.CreateTime,
	}, nil
}

// Verify performs a wrap+unwrap round-trip to confirm encryption permissions.
func (p *GCPKMSProvider) Verify(ctx context.Context, keyRef string) error {
	testPlain := make([]byte, 32)
	for i := range testPlain {
		testPlain[i] = byte(i)
	}
	wrapped, err := p.WrapKey(ctx, testPlain, keyRef)
	if err != nil {
		return fmt.Errorf("gcp kms verify wrap: %w", err)
	}
	recovered, err := p.UnwrapKey(ctx, wrapped, keyRef)
	if err != nil {
		return fmt.Errorf("gcp kms verify unwrap: %w", err)
	}
	for i, b := range recovered {
		if b != testPlain[i] {
			return fmt.Errorf("gcp kms verify: round-trip mismatch at byte %d", i)
		}
	}
	return nil
}
