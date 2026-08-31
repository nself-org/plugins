package internal

import "time"

// Verification represents an idme_verifications row.
type Verification struct {
	ID               string     `json:"id"`
	SourceAccountID  string     `json:"source_account_id"`
	UserID           string     `json:"user_id"`
	IDmeUUID         *string    `json:"idme_uuid,omitempty"`
	Email            string     `json:"email"`
	Verified         bool       `json:"verified"`
	VerificationLevel *string   `json:"verification_level,omitempty"`
	FirstName        *string    `json:"first_name,omitempty"`
	LastName         *string    `json:"last_name,omitempty"`
	BirthDate        *time.Time `json:"birth_date,omitempty"`
	Zip              *string    `json:"zip,omitempty"`
	Phone            *string    `json:"phone,omitempty"`
	AccessToken      *string    `json:"access_token,omitempty"`
	RefreshToken     *string    `json:"refresh_token,omitempty"`
	TokenExpiresAt   *time.Time `json:"token_expires_at,omitempty"`
	VerifiedAt       *time.Time `json:"verified_at,omitempty"`
	LastSyncedAt     time.Time  `json:"last_synced_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// Group represents an idme_groups row.
type Group struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	VerificationID  string     `json:"verification_id"`
	UserID          string     `json:"user_id"`
	GroupType       string     `json:"group_type"`
	GroupName       string     `json:"group_name"`
	Verified        bool       `json:"verified"`
	VerifiedAt      *time.Time `json:"verified_at,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	Affiliation     *string    `json:"affiliation,omitempty"`
	Rank            *string    `json:"rank,omitempty"`
	Status          *string    `json:"status,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Badge represents an idme_badges row.
type Badge struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	VerificationID  string     `json:"verification_id"`
	UserID          string     `json:"user_id"`
	BadgeType       string     `json:"badge_type"`
	BadgeName       string     `json:"badge_name"`
	BadgeIcon       *string    `json:"badge_icon,omitempty"`
	BadgeColor      *string    `json:"badge_color,omitempty"`
	VerifiedAt      *time.Time `json:"verified_at,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	Active          bool       `json:"active"`
	DisplayOrder    int        `json:"display_order"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Attribute represents an idme_attributes row.
type Attribute struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	VerificationID  string     `json:"verification_id"`
	UserID          string     `json:"user_id"`
	AttributeKey    string     `json:"attribute_key"`
	AttributeValue  *string    `json:"attribute_value,omitempty"`
	AttributeType   *string    `json:"attribute_type,omitempty"`
	Verified        bool       `json:"verified"`
	VerifiedAt      *time.Time `json:"verified_at,omitempty"`
	Source          string     `json:"source"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// WebhookEvent represents an idme_webhook_events row.
type WebhookEvent struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	EventID         *string    `json:"event_id,omitempty"`
	EventType       string     `json:"event_type"`
	UserID          *string    `json:"user_id,omitempty"`
	VerificationID  *string    `json:"verification_id,omitempty"`
	Processed       bool       `json:"processed"`
	ProcessedAt     *time.Time `json:"processed_at,omitempty"`
	Error           *string    `json:"error,omitempty"`
	RetryCount      int        `json:"retry_count"`
	CreatedAt       time.Time  `json:"created_at"`
	ReceivedAt      time.Time  `json:"received_at"`
}

// Stats is the response for GET /api/v1/stats.
type Stats struct {
	TotalVerifications int `json:"total_verifications"`
	VerifiedCount      int `json:"verified_count"`
	TotalGroups        int `json:"total_groups"`
	TotalBadges        int `json:"total_badges"`
	TotalWebhookEvents int `json:"total_webhook_events"`
}
