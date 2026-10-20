package checks

// Purpose: the App's RS256 JWT, from the standard library only.
// Inputs:  the App id, the parsed private key, a clock reading.
// Outputs: a compact JWT that GitHub accepts for POST /app/installations/{id}/access_tokens.
// Constraints: iat is 60 s in the past (clock drift) and exp 9 minutes ahead (GitHub allows 10).
// Parse and sign errors say what kind of thing failed, never any key material. The JWT is
// short-lived, used once per token exchange and never logged or stored.

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"time"
)

// ParseKey reads an RSA private key in PKCS#1 or PKCS#8 PEM form.
func ParseKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, coded("the App key is not PEM")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, coded("the App key is not an RSA private key")
	}
	k, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, coded("the App key is not an RSA private key")
	}
	return k, nil
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// SignJWT signs the App JWT for appID at now. The signature is deterministic (PKCS#1 v1.5).
func SignJWT(appID string, key *rsa.PrivateKey, now time.Time) (string, error) {
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{"iat": now.Add(-60 * time.Second).Unix(), "exp": now.Add(9 * time.Minute).Unix(), "iss": appID})
	signing := b64(header) + "." + b64(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", coded("the App JWT could not be signed")
	}
	return signing + "." + b64(sig), nil
}
