package internal

import "time"

// Plan represents a subscription plan.
type Plan struct {
	ID              string          `json:"id"`
	SourceAccountID string          `json:"source_account_id"`
	Name            string          `json:"name"`
	Slug            string          `json:"slug"`
	Description     *string         `json:"description,omitempty"`
	BillingInterval string          `json:"billing_interval"`
	PriceCents      int             `json:"price_cents"`
	Currency        string          `json:"currency"`
	TrialDays       int             `json:"trial_days"`
	PlanType        string          `json:"plan_type"`
	IsPublic        bool            `json:"is_public"`
	IsArchived      bool            `json:"is_archived"`
	Features        interface{}     `json:"features"`
	Quotas          interface{}     `json:"quotas"`
	Metadata        interface{}     `json:"metadata,omitempty"`
	DisplayOrder    int             `json:"display_order"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// Subscription represents a user or workspace subscription.
type Subscription struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	WorkspaceID     *string    `json:"workspace_id,omitempty"`
	UserID          *string    `json:"user_id,omitempty"`
	PlanID          string     `json:"plan_id"`
	Status          string     `json:"status"`
	BillingInterval string     `json:"billing_interval"`
	PriceCents      int        `json:"price_cents"`
	Currency        string     `json:"currency"`
	TrialStart      *time.Time `json:"trial_start,omitempty"`
	TrialEnd        *time.Time `json:"trial_end,omitempty"`
	PeriodStart     time.Time  `json:"current_period_start"`
	PeriodEnd       time.Time  `json:"current_period_end"`
	CancelAtEnd     bool       `json:"cancel_at_period_end"`
	CanceledAt      *time.Time `json:"canceled_at,omitempty"`
	Metadata        interface{} `json:"metadata,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Feature represents a named feature definition.
type Feature struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	Key             string    `json:"key"`
	Name            string    `json:"name"`
	Description     *string   `json:"description,omitempty"`
	FeatureType     string    `json:"feature_type"`
	IsActive        bool      `json:"is_active"`
	Category        *string   `json:"category,omitempty"`
	Metadata        interface{} `json:"metadata,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Quota represents a usage quota tied to a plan and feature.
type Quota struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	PlanID          string    `json:"plan_id"`
	FeatureID       string    `json:"feature_id"`
	LimitValue      *int      `json:"limit_value,omitempty"`
	Period          string    `json:"period"`
	CreatedAt       time.Time `json:"created_at"`
}

// Usage records a single usage event for a quota.
type Usage struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	UserID          *string   `json:"user_id,omitempty"`
	QuotaKey        string    `json:"quota_key"`
	UsageAmount     int       `json:"usage_amount"`
	CreatedAt       time.Time `json:"created_at"`
}

// Addon represents an addon subscription attached to a base subscription.
type Addon struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	SubscriptionID  string    `json:"subscription_id"`
	AddonPlanID     string    `json:"addon_plan_id"`
	Quantity        int       `json:"quantity"`
	PriceCents      int       `json:"price_cents"`
	Currency        string    `json:"currency"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
