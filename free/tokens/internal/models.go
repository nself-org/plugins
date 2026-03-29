package internal

import (
	"encoding/json"
	"time"
)

// ============================================================================
// Database Record Types
// ============================================================================

// SigningKey represents a row in np_tokens_signing_keys.
type SigningKey struct {
	ID                   string     `json:"id"`
	SourceAccountID      string     `json:"source_account_id"`
	Name                 string     `json:"name"`
	Algorithm            string     `json:"algorithm"`
	KeyMaterialEncrypted string     `json:"-"`
	IsActive             bool       `json:"is_active"`
	RotatedFrom          *string    `json:"rotated_from"`
	CreatedAt            time.Time  `json:"created_at"`
	RotatedAt            *time.Time `json:"rotated_at"`
	ExpiresAt            *time.Time `json:"expires_at"`
}

// IssuedToken represents a row in np_tokens_issued.
type IssuedToken struct {
	ID              string          `json:"id"`
	SourceAccountID string          `json:"source_account_id"`
	TokenHash       string          `json:"-"`
	TokenType       string          `json:"token_type"`
	SigningKeyID    *string         `json:"signing_key_id"`
	UserID          string          `json:"user_id"`
	DeviceID        *string         `json:"device_id"`
	ContentID       string          `json:"content_id"`
	ContentType     *string         `json:"content_type"`
	Permissions     json.RawMessage `json:"permissions"`
	IPAddress       *string         `json:"ip_address"`
	IssuedAt        time.Time       `json:"issued_at"`
	ExpiresAt       time.Time       `json:"expires_at"`
	Revoked         bool            `json:"revoked"`
	RevokedAt       *time.Time      `json:"revoked_at"`
	RevokedReason   *string         `json:"revoked_reason"`
	LastUsedAt      *time.Time      `json:"last_used_at"`
	UseCount        int             `json:"use_count"`
}

// EncryptionKey represents a row in np_tokens_encryption_keys.
type EncryptionKey struct {
	ID                   string     `json:"id"`
	SourceAccountID      string     `json:"source_account_id"`
	ContentID            string     `json:"content_id"`
	KeyMaterialEncrypted string     `json:"-"`
	KeyIV                string     `json:"key_iv"`
	KeyURI               string     `json:"key_uri"`
	RotationGeneration   int        `json:"rotation_generation"`
	IsActive             bool       `json:"is_active"`
	CreatedAt            time.Time  `json:"created_at"`
	RotatedAt            *time.Time `json:"rotated_at"`
	ExpiresAt            *time.Time `json:"expires_at"`
}

// Entitlement represents a row in np_tokens_entitlements.
type Entitlement struct {
	ID              string          `json:"id"`
	SourceAccountID string          `json:"source_account_id"`
	UserID          string          `json:"user_id"`
	ContentID       string          `json:"content_id"`
	ContentType     *string         `json:"content_type"`
	EntitlementType string          `json:"entitlement_type"`
	GrantedBy       string          `json:"granted_by"`
	GrantedAt       time.Time       `json:"granted_at"`
	ExpiresAt       *time.Time      `json:"expires_at"`
	Revoked         bool            `json:"revoked"`
	Metadata        json.RawMessage `json:"metadata"`
}

// ============================================================================
// API Request Types
// ============================================================================

// IssueTokenRequest is the JSON body for POST /v1/tokens/issue.
type IssueTokenRequest struct {
	UserID        string                 `json:"userId"`
	DeviceID      string                 `json:"deviceId,omitempty"`
	ContentID     string                 `json:"contentId"`
	ContentType   string                 `json:"contentType,omitempty"`
	TokenType     string                 `json:"tokenType,omitempty"`
	TTLSeconds    *int                   `json:"ttlSeconds,omitempty"`
	Permissions   map[string]interface{} `json:"permissions,omitempty"`
	IPRestriction string                 `json:"ipRestriction,omitempty"`
}

// ValidateTokenRequest is the JSON body for POST /v1/tokens/validate.
type ValidateTokenRequest struct {
	Token     string `json:"token"`
	ContentID string `json:"contentId,omitempty"`
	IPAddress string `json:"ipAddress,omitempty"`
}

// RevokeTokenRequest is the JSON body for POST /v1/tokens/revoke.
type RevokeTokenRequest struct {
	TokenID string `json:"tokenId"`
	Reason  string `json:"reason,omitempty"`
}

// RevokeUserTokensRequest is the JSON body for POST /v1/tokens/revoke with userId.
type RevokeUserTokensRequest struct {
	UserID string `json:"userId"`
	Reason string `json:"reason,omitempty"`
}

// RevokeContentTokensRequest is the JSON body for POST /v1/tokens/revoke with contentId.
type RevokeContentTokensRequest struct {
	ContentID string `json:"contentId"`
	Reason    string `json:"reason,omitempty"`
}

// CreateSigningKeyRequest is the JSON body for POST /v1/keys.
type CreateSigningKeyRequest struct {
	Name      string `json:"name"`
	Algorithm string `json:"algorithm,omitempty"`
}

// RotateKeyRequest is the JSON body for POST /v1/keys/rotate.
type RotateKeyRequest struct {
	ExpireOldAfterHours *int `json:"expireOldAfterHours,omitempty"`
}

// CreateEncryptionKeyRequest is the JSON body for POST /v1/encryption/keys.
type CreateEncryptionKeyRequest struct {
	ContentID string `json:"contentId"`
}

// RotateEncryptionKeyRequest is the JSON body for POST /v1/encryption/keys/:contentId/rotate.
type RotateEncryptionKeyRequest struct {
	ExpireOldAfterHours *int `json:"expireOldAfterHours,omitempty"`
}

// CheckEntitlementRequest is the JSON body for POST /v1/entitlements/check.
type CheckEntitlementRequest struct {
	UserID          string `json:"userId"`
	ContentID       string `json:"contentId"`
	EntitlementType string `json:"entitlementType"`
	DeviceID        string `json:"deviceId,omitempty"`
}

// GrantEntitlementRequest is the JSON body for POST /v1/entitlements.
type GrantEntitlementRequest struct {
	UserID          string                 `json:"userId"`
	ContentID       string                 `json:"contentId"`
	ContentType     string                 `json:"contentType,omitempty"`
	EntitlementType string                 `json:"entitlementType"`
	ExpiresAt       string                 `json:"expiresAt,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// RevokeEntitlementRequest is the JSON body for DELETE /v1/entitlements.
type RevokeEntitlementRequest struct {
	UserID          string `json:"userId"`
	ContentID       string `json:"contentId"`
	EntitlementType string `json:"entitlementType"`
}

// ============================================================================
// API Response Types
// ============================================================================

// IssueTokenResponse is returned from POST /v1/tokens/issue.
type IssueTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt"`
	TokenID   string `json:"tokenId"`
}

// ValidateTokenResponse is returned from POST /v1/tokens/validate.
type ValidateTokenResponse struct {
	Valid       bool                   `json:"valid"`
	UserID      string                 `json:"userId,omitempty"`
	ContentID   string                 `json:"contentId,omitempty"`
	Permissions map[string]interface{} `json:"permissions,omitempty"`
	ExpiresAt   string                 `json:"expiresAt,omitempty"`
}

// CheckEntitlementResponse is returned from POST /v1/entitlements/check.
type CheckEntitlementResponse struct {
	Allowed      bool                   `json:"allowed"`
	Reason       string                 `json:"reason,omitempty"`
	Restrictions map[string]interface{} `json:"restrictions,omitempty"`
	ExpiresAt    string                 `json:"expiresAt,omitempty"`
}

// TokensStats holds aggregate counts for the stats endpoint.
type TokensStats struct {
	TotalSigningKeys   int `json:"totalSigningKeys"`
	ActiveSigningKeys  int `json:"activeSigningKeys"`
	TotalTokensIssued  int `json:"totalTokensIssued"`
	ActiveTokens       int `json:"activeTokens"`
	RevokedTokens      int `json:"revokedTokens"`
	ExpiredTokens      int `json:"expiredTokens"`
	TotalEncryptionKeys int `json:"totalEncryptionKeys"`
	TotalEntitlements  int `json:"totalEntitlements"`
	ActiveEntitlements int `json:"activeEntitlements"`
}

// InsertTokenParams gathers all fields needed to insert an issued token.
type InsertTokenParams struct {
	TokenHash    string
	TokenType    string
	SigningKeyID string
	UserID       string
	DeviceID     *string
	ContentID    string
	ContentType  *string
	Permissions  map[string]interface{}
	IPAddress    *string
	ExpiresAt    time.Time
}

// GrantEntitlementParams gathers all fields needed to insert an entitlement.
type GrantEntitlementParams struct {
	UserID          string
	ContentID       string
	ContentType     *string
	EntitlementType string
	ExpiresAt       *time.Time
	Metadata        map[string]interface{}
	GrantedBy       string
}
