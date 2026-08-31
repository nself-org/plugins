package kms

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// VaultKMSProvider wraps the HashiCorp Vault Transit secrets engine for
// envelope encryption. Key references are the key name in the transit mount,
// e.g. "transit/keys/tenant-abc". The mount path is embedded in keyRef.
type VaultKMSProvider struct {
	endpoint string // e.g. "https://vault.example.com"
	token    string // Vault token or agent-provided token
	client   *http.Client
}

// NewVaultKMSProvider constructs a VaultKMSProvider.
// endpoint: full Vault address, e.g. "https://vault.example.com"
// token:    Vault token with encrypt/decrypt policy on the transit mount.
func NewVaultKMSProvider(endpoint, token string) *VaultKMSProvider {
	return &VaultKMSProvider{
		endpoint: strings.TrimRight(endpoint, "/"),
		token:    token,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

type vaultEncryptReq struct {
	Plaintext string `json:"plaintext"` // base64-encoded
}

type vaultEncryptResp struct {
	Data struct {
		Ciphertext string `json:"ciphertext"`
	} `json:"data"`
	Errors []string `json:"errors,omitempty"`
}

type vaultDecryptReq struct {
	Ciphertext string `json:"ciphertext"`
}

type vaultDecryptResp struct {
	Data struct {
		Plaintext string `json:"plaintext"` // base64-encoded
	} `json:"data"`
	Errors []string `json:"errors,omitempty"`
}

// keyName extracts "transit/keys/mykey" → "mykey" and the mount path.
// keyRef format: "<mount>/encrypt/<keyname>" or just "<keyname>".
// We accept either "transit/keys/foo" (Vault UI style) or "foo" (bare key name)
// and normalize to the encrypt/decrypt path under /v1/transit/.
func (p *VaultKMSProvider) encryptPath(keyRef string) string {
	// Accept "transit/keys/foo" → use /v1/transit/encrypt/foo
	name := keyRef
	if idx := strings.LastIndex(keyRef, "/"); idx >= 0 {
		name = keyRef[idx+1:]
	}
	return fmt.Sprintf("%s/v1/transit/encrypt/%s", p.endpoint, name)
}

func (p *VaultKMSProvider) decryptPath(keyRef string) string {
	name := keyRef
	if idx := strings.LastIndex(keyRef, "/"); idx >= 0 {
		name = keyRef[idx+1:]
	}
	return fmt.Sprintf("%s/v1/transit/decrypt/%s", p.endpoint, name)
}

// WrapKey calls the Vault Transit encrypt endpoint.
func (p *VaultKMSProvider) WrapKey(ctx context.Context, plainDEK []byte, keyRef string) ([]byte, error) {
	b64 := base64.StdEncoding.EncodeToString(plainDEK)
	body, _ := json.Marshal(vaultEncryptReq{Plaintext: b64})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.encryptPath(keyRef), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("vault kms: build encrypt request: %w", err)
	}
	req.Header.Set("X-Vault-Token", p.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault kms: encrypt request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vault kms: encrypt HTTP %d: %s", resp.StatusCode, raw)
	}

	var out vaultEncryptResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("vault kms: decode encrypt response: %w", err)
	}
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("vault kms: encrypt errors: %s", strings.Join(out.Errors, "; "))
	}
	return []byte(out.Data.Ciphertext), nil
}

// UnwrapKey calls the Vault Transit decrypt endpoint. Returns an error when the
// key is revoked or the caller lacks decrypt permission — no fallback.
func (p *VaultKMSProvider) UnwrapKey(ctx context.Context, wrappedDEK []byte, keyRef string) ([]byte, error) {
	body, _ := json.Marshal(vaultDecryptReq{Ciphertext: string(wrappedDEK)})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.decryptPath(keyRef), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("vault kms: build decrypt request: %w", err)
	}
	req.Header.Set("X-Vault-Token", p.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault kms: decrypt request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("vault kms: permission denied (CMK revoked or token invalid)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vault kms: decrypt HTTP %d: %s", resp.StatusCode, raw)
	}

	var out vaultDecryptResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("vault kms: decode decrypt response: %w", err)
	}
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("vault kms: decrypt errors: %s", strings.Join(out.Errors, "; "))
	}
	b64plain := out.Data.Plaintext
	plain, err := base64.StdEncoding.DecodeString(b64plain)
	if err != nil {
		return nil, fmt.Errorf("vault kms: decode plaintext base64: %w", err)
	}
	return plain, nil
}

// DescribeKey reads the key metadata from the Transit mount.
func (p *VaultKMSProvider) DescribeKey(ctx context.Context, keyRef string) (KeyMetadata, error) {
	name := keyRef
	if idx := strings.LastIndex(keyRef, "/"); idx >= 0 {
		name = keyRef[idx+1:]
	}
	url := fmt.Sprintf("%s/v1/transit/keys/%s", p.endpoint, name)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return KeyMetadata{}, fmt.Errorf("vault kms: describe key request: %w", err)
	}
	req.Header.Set("X-Vault-Token", p.token)

	resp, err := p.client.Do(req)
	if err != nil {
		return KeyMetadata{}, fmt.Errorf("vault kms: describe key: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return KeyMetadata{}, fmt.Errorf("vault kms: describe key HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Data struct {
			Type string `json:"type"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return KeyMetadata{}, fmt.Errorf("vault kms: decode describe response: %w", err)
	}
	return KeyMetadata{
		KeyRef:    keyRef,
		Provider:  ProviderVault,
		Algorithm: payload.Data.Type,
		State:     "ENABLED",
	}, nil
}

// Verify performs a wrap+unwrap round-trip using the Transit engine.
func (p *VaultKMSProvider) Verify(ctx context.Context, keyRef string) error {
	testPlain := make([]byte, 32)
	for i := range testPlain {
		testPlain[i] = byte(i)
	}
	wrapped, err := p.WrapKey(ctx, testPlain, keyRef)
	if err != nil {
		return fmt.Errorf("vault kms verify wrap: %w", err)
	}
	recovered, err := p.UnwrapKey(ctx, wrapped, keyRef)
	if err != nil {
		return fmt.Errorf("vault kms verify unwrap: %w", err)
	}
	for i, b := range recovered {
		if b != testPlain[i] {
			return fmt.Errorf("vault kms verify: round-trip mismatch at byte %d", i)
		}
	}
	return nil
}
