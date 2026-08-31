package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ExpectedAudience is the required value of the JWT `aud` claim for vault-scoped tokens.
// Tokens issued for other services (e.g. nself-sync) MUST NOT validate against this aud
// — this prevents cross-service token reuse (V05-F3).
const ExpectedAudience = "nself-vault"

// Claims holds the nSelf JWT claims relevant to the vault service.
type Claims struct {
	jwt.RegisteredClaims
	UserID   string `json:"user_id"`
	TenantID string `json:"tenant_id,omitempty"`
	Role     string `json:"role,omitempty"`
	// Hasura-style claims namespace (optional — present when token originates from Hasura)
	HasuraClaims *HasuraClaims `json:"https://hasura.io/jwt/claims,omitempty"`
}

// HasuraClaims mirrors the Hasura JWT claims namespace.
type HasuraClaims struct {
	UserID   string `json:"X-Hasura-User-Id"`
	TenantID string `json:"X-Hasura-Tenant-Id,omitempty"`
	Role     string `json:"X-Hasura-Default-Role,omitempty"`
}

// UserInfo is the resolved identity extracted from a validated JWT.
type UserInfo struct {
	UserID   string
	TenantID string // empty string means no Cloud tenant (self-hosted single-tenant)
	Role     string
}

// Extractor validates a Bearer JWT and extracts the user identity.
// It checks both top-level claims and the Hasura claims namespace.
type Extractor struct {
	secret []byte
}

// NewExtractor creates an Extractor using the provided HMAC secret.
func NewExtractor(secret string) *Extractor {
	return &Extractor{secret: []byte(secret)}
}

// FromRequest extracts and validates the JWT from the Authorization header.
// Returns (UserInfo, nil) on success, or a descriptive error on failure.
func (e *Extractor) FromRequest(r *http.Request) (UserInfo, error) {
	raw, err := bearerToken(r)
	if err != nil {
		return UserInfo{}, err
	}
	return e.Parse(raw)
}

// Parse validates a raw JWT string and returns the extracted UserInfo.
//
// V05-F3: enforces two security requirements beyond signature validity:
//   1. The `aud` claim MUST contain ExpectedAudience ("nself-vault"). Tokens
//      issued for other services (nself-sync, hasura, custom plugins) are
//      rejected even when signed with the same secret. This prevents
//      cross-service token reuse.
//   2. The `exp` claim MUST be present AND in the future. Tokens without an
//      expiration are rejected — never accept eternal tokens against the vault.
func (e *Extractor) Parse(raw string) (UserInfo, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("vault/auth: unexpected signing method: %v", t.Header["alg"])
		}
		return e.secret, nil
	})
	if err != nil {
		return UserInfo{}, fmt.Errorf("vault/auth: invalid token: %w", err)
	}

	// V05-F3a: Mandatory `exp` claim. Reject tokens that omit expiration entirely.
	// jwt/v5 enforces "if present, must be in the future" — but does NOT require
	// presence. The vault explicitly requires presence.
	if claims.ExpiresAt == nil {
		return UserInfo{}, errors.New("vault/auth: token missing required exp claim")
	}
	if claims.ExpiresAt.Time.Before(time.Now()) {
		return UserInfo{}, errors.New("vault/auth: token expired")
	}

	// V05-F3b: Mandatory `aud` claim matching ExpectedAudience.
	// Reject tokens scoped to other services (cross-service token reuse).
	if !hasAudience(claims.Audience, ExpectedAudience) {
		return UserInfo{}, fmt.Errorf("vault/auth: token aud does not include %q", ExpectedAudience)
	}

	info := UserInfo{
		UserID:   claims.UserID,
		TenantID: claims.TenantID,
		Role:     claims.Role,
	}

	// Prefer Hasura claims namespace when present (tokens from Hasura Actions carry this).
	if hc := claims.HasuraClaims; hc != nil {
		if hc.UserID != "" {
			info.UserID = hc.UserID
		}
		if hc.TenantID != "" {
			info.TenantID = hc.TenantID
		}
		if hc.Role != "" {
			info.Role = hc.Role
		}
	}

	if info.UserID == "" {
		return UserInfo{}, errors.New("vault/auth: token missing user_id claim")
	}
	return info, nil
}

// hasAudience reports whether the JWT `aud` claim includes the expected value.
// JWT `aud` may be a single string or an array of strings — both forms are accepted
// here. The match is exact (no wildcards) and case-sensitive.
func hasAudience(aud jwt.ClaimStrings, expected string) bool {
	for _, a := range aud {
		if a == expected {
			return true
		}
	}
	return false
}

// bearerToken extracts the token string from "Authorization: Bearer <token>".
func bearerToken(r *http.Request) (string, error) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", errors.New("vault/auth: missing Authorization header")
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return "", errors.New("vault/auth: Authorization header must use Bearer scheme")
	}
	tok := strings.TrimPrefix(h, prefix)
	if tok == "" {
		return "", errors.New("vault/auth: empty Bearer token")
	}
	return tok, nil
}
