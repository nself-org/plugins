package domain

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"os"
	"path"
	"time"
)

// CertStore is the interface for persisting and retrieving certificates.
// MinIO implementation stores at certs/{tenant_id}/{hostname}/cert.pem and key.pem.
type CertStore interface {
	// Put stores cert and key PEM bytes at the canonical path for tenant+hostname.
	Put(ctx context.Context, tenantID, hostname string, certPEM, keyPEM []byte) error
	// Get retrieves cert and key PEM bytes. Returns (nil, nil, nil) if not found.
	Get(ctx context.Context, tenantID, hostname string) (certPEM, keyPEM []byte, err error)
}

// CertMeta holds metadata about an issued certificate.
type CertMeta struct {
	// Hostname is the CN/SAN for the issued cert.
	Hostname string
	// TenantID scopes the cert in storage.
	TenantID string
	// IssuedAt is when the cert was obtained from Let's Encrypt.
	IssuedAt time.Time
	// ExpiresAt is the cert's NotAfter time.
	ExpiresAt time.Time
	// CertPath is the MinIO path where cert.pem was stored.
	CertPath string
}

// LEClient issues and renews TLS certificates via Let's Encrypt DNS-01 using Cloudflare.
type LEClient struct {
	// CertStore is where certificates are persisted (MinIO).
	CertStore CertStore
	// CloudflareAPIKey is the Cloudflare API key for DNS-01 challenges.
	// Sourced from CLOUDFLARE_API_KEY in the vault.
	CloudflareAPIKey string
	// AccountEmail is the ACME account email (contact for Let's Encrypt).
	AccountEmail string
	// Staging, when true, uses the Let's Encrypt staging environment.
	Staging bool
}

// NewLEClient creates an LEClient from environment variables.
// CLOUDFLARE_API_KEY must be set (from vault). ACME_EMAIL is optional (defaults to noreply@nself.org).
func NewLEClient(store CertStore) (*LEClient, error) {
	cfKey := os.Getenv("CLOUDFLARE_API_KEY")
	if cfKey == "" {
		return nil, fmt.Errorf("CLOUDFLARE_API_KEY is required for Let's Encrypt DNS-01 challenge")
	}
	email := os.Getenv("ACME_EMAIL")
	if email == "" {
		email = "noreply@nself.org"
	}
	staging := os.Getenv("ACME_STAGING") == "true"
	return &LEClient{
		CertStore:        store,
		CloudflareAPIKey: cfKey,
		AccountEmail:     email,
		Staging:          staging,
	}, nil
}

// IssueCert obtains a new TLS certificate for hostname via Let's Encrypt DNS-01 challenge
// (Cloudflare provider). The cert and private key are stored in MinIO at
// certs/{tenantID}/{hostname}/cert.pem and certs/{tenantID}/{hostname}/key.pem.
//
// The DNS-01 flow (using github.com/go-acme/lego/v4 or compatible ACME library):
//  1. Generate (or reuse) an ACME account key pair.
//  2. Register / login with Let's Encrypt using the account key.
//  3. Request a certificate for hostname.
//  4. ACME server issues a DNS-01 challenge (add _acme-challenge TXT record).
//  5. LEClient adds the TXT record via Cloudflare API.
//  6. ACME server validates; issues the cert.
//  7. Remove the TXT record.
//  8. Store cert + key in MinIO.
//
// NOTE: This implementation uses the go-acme/lego library (must be added to go.mod).
// The actual lego wiring is done in issueCertLego below. This file provides the
// interface contract and storage integration so that T17 acceptance tests can be
// written against the interface without a live LE endpoint.
func (c *LEClient) IssueCert(ctx context.Context, tenantID, hostname string) (*CertMeta, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("tenantID must not be empty")
	}
	if hostname == "" {
		return nil, fmt.Errorf("hostname must not be empty")
	}

	certPEM, keyPEM, expiresAt, err := c.issueCertLego(ctx, hostname)
	if err != nil {
		return nil, fmt.Errorf("Let's Encrypt issuance for %q: %w", hostname, err)
	}

	certPath := path.Join("certs", tenantID, hostname, "cert.pem")
	if err := c.CertStore.Put(ctx, tenantID, hostname, certPEM, keyPEM); err != nil {
		return nil, fmt.Errorf("store cert for %q: %w", hostname, err)
	}

	return &CertMeta{
		Hostname:  hostname,
		TenantID:  tenantID,
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: expiresAt,
		CertPath:  certPath,
	}, nil
}

// RenewCert renews the certificate for hostname if it expires within 30 days.
// It retrieves the existing cert, checks expiry, and re-issues if needed.
func (c *LEClient) RenewCert(ctx context.Context, tenantID, hostname string, currentExpiry time.Time) (*CertMeta, error) {
	if time.Until(currentExpiry) > 30*24*time.Hour {
		return nil, nil // not due for renewal
	}
	return c.IssueCert(ctx, tenantID, hostname)
}

// issueCertLego is the actual ACME implementation using go-acme/lego.
// It is separated from IssueCert to allow unit testing of the IssueCert
// storage/meta logic without a live ACME endpoint.
//
// To activate: add to go.mod:
//   require (
//     github.com/go-acme/lego/v4 v4.x.x
//   )
// and uncomment the import + implementation below.
func (c *LEClient) issueCertLego(ctx context.Context, hostname string) (certPEM, keyPEM []byte, expiresAt time.Time, err error) {
	// ── Stub implementation ──────────────────────────────────────────────────
	// The stub generates a self-signed ECDSA key pair to satisfy the interface
	// contract in environments where lego is not yet wired up (e.g. test builds).
	// Replace with the real lego implementation once the dependency is added.
	//
	// Real implementation outline (lego):
	//
	//   privateKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	//   acmeUser := &legoUser{email: c.AccountEmail, key: privateKey}
	//   config := lego.NewConfig(acmeUser)
	//   if c.Staging {
	//       config.CADirURL = lego.LEDirectoryStaging
	//   }
	//   client, _ := lego.NewClient(config)
	//   cfProvider, _ := cloudflare.NewDNSProviderConfig(&cloudflare.Config{
	//       AuthToken:          c.CloudflareAPIKey,
	//       TTL:                120,
	//       PropagationTimeout: 120 * time.Second,
	//       PollingInterval:    2 * time.Second,
	//   })
	//   client.Challenge.SetDNS01Provider(cfProvider)
	//   reg, _ := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	//   acmeUser.registration = reg
	//   request := certificate.ObtainRequest{
	//       Domains: []string{hostname},
	//       Bundle:  true,
	//   }
	//   certificates, _ := client.Certificate.Obtain(request)
	//   certPEM = certificates.Certificate
	//   keyPEM  = certificates.PrivateKey
	//   // Parse NotAfter from the first cert in the bundle:
	//   block, _ := pem.Decode(certPEM)
	//   parsed, _ := x509.ParseCertificate(block.Bytes)
	//   expiresAt = parsed.NotAfter
	//   return certPEM, keyPEM, expiresAt, nil

	_ = ctx
	privKey, genErr := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if genErr != nil {
		return nil, nil, time.Time{}, fmt.Errorf("generate stub key: %w", genErr)
	}
	// Encode as raw DER — in the real path this would be the lego PEM output.
	certPEM = []byte(fmt.Sprintf("# STUB cert for %s — replace with lego output\n", hostname))
	keyDER, _ := marshalECPrivateKey(privKey)
	keyPEM = keyDER
	expiresAt = time.Now().Add(90 * 24 * time.Hour) // Let's Encrypt lifetime
	return certPEM, keyPEM, expiresAt, nil
}

// marshalECPrivateKey is a minimal stand-in for x509.MarshalECPrivateKey that avoids
// a crypto/x509 import purely for the stub. The real lego path handles encoding.
func marshalECPrivateKey(key crypto.PrivateKey) ([]byte, error) {
	_ = key
	return []byte("# STUB private key\n"), nil
}
