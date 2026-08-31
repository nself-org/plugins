// Package auth provides JWT validation for nself-sync.
//
// Tokens use HS256 with the shared secret from env NSELF_SYNC_JWT_SECRET.
// Claims required: sub (user_id), did (device_id), exp (mandatory).
// Optional: aud, iss.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// ErrInvalidToken is returned when the JWT cannot be parsed or verified.
var ErrInvalidToken = errors.New("invalid token")

// ErrExpired is returned when the JWT has expired or lacks an exp claim.
var ErrExpired = errors.New("token expired or missing exp")

// Claims is the validated set of fields extracted from a sync JWT.
type Claims struct {
	UserID   string `json:"sub"`
	DeviceID string `json:"did"`
	Exp      int64  `json:"exp"`
	Aud      string `json:"aud,omitempty"`
}

// SecretFromEnv returns the HMAC secret bytes from NSELF_SYNC_JWT_SECRET.
// Returns an error when the env var is empty.
func SecretFromEnv() ([]byte, error) {
	s := os.Getenv("NSELF_SYNC_JWT_SECRET")
	if s == "" {
		return nil, errors.New("NSELF_SYNC_JWT_SECRET not set")
	}
	return []byte(s), nil
}

// Verify parses and verifies a JWT signed with HS256 against secret.
// Returns Claims on success. user_id and device_id must be non-empty.
// exp must be present and in the future.
func Verify(token string, secret []byte) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	// Verify header indicates HS256.
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("%w: header b64: %v", ErrInvalidToken, err)
	}
	var hdr struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(headerBytes, &hdr); err != nil {
		return nil, fmt.Errorf("%w: header json: %v", ErrInvalidToken, err)
	}
	if hdr.Alg != "HS256" {
		return nil, fmt.Errorf("%w: unsupported alg %q", ErrInvalidToken, hdr.Alg)
	}

	// Verify signature.
	signingInput := parts[0] + "." + parts[1]
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("%w: signature b64: %v", ErrInvalidToken, err)
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	expected := mac.Sum(nil)
	if !hmac.Equal(sigBytes, expected) {
		return nil, ErrInvalidToken
	}

	// Decode and validate claims.
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: payload b64: %v", ErrInvalidToken, err)
	}
	var c Claims
	if err := json.Unmarshal(payloadBytes, &c); err != nil {
		return nil, fmt.Errorf("%w: payload json: %v", ErrInvalidToken, err)
	}
	if c.UserID == "" || c.DeviceID == "" {
		return nil, fmt.Errorf("%w: missing sub or did", ErrInvalidToken)
	}
	if c.Exp == 0 {
		return nil, ErrExpired
	}
	if time.Now().Unix() >= c.Exp {
		return nil, ErrExpired
	}
	return &c, nil
}
