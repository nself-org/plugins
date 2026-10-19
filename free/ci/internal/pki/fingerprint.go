package pki

// Purpose: the one fingerprint form of the pairing flow (Epic D5): "SHA256:" plus the unpadded standard
//          base64 of a SHA-256 digest, the OpenSSH form.
// Inputs:  a certificate (root) or a CSR / certificate public key (SPKI).
// Outputs: the full 256-bit string; never truncated, never words.
// Constraints: stdlib only; Equal compares in constant time.

import (
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
)

const fpPrefix = "SHA256:"

func digestString(b []byte) string {
	sum := sha256.Sum256(b)
	return fpPrefix + base64.RawStdEncoding.EncodeToString(sum[:])
}

// CertFingerprint is the fingerprint of a certificate's DER bytes (the root fingerprint).
func CertFingerprint(der []byte) string { return digestString(der) }

// KeyFingerprint is the fingerprint of a public key's SPKI DER (the CSR key fingerprint).
func KeyFingerprint(pub any) (string, error) {
	spki, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", err
	}
	return digestString(spki), nil
}

// SPKIFingerprint is the fingerprint of SPKI DER bytes already in hand.
func SPKIFingerprint(spki []byte) string { return digestString(spki) }

// Equal reports whether two fingerprint strings are identical (constant time).
func Equal(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
