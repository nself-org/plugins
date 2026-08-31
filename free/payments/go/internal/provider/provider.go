// Package provider defines the unified Provider interface for all payment backends.
package provider

import (
	"context"
	"time"
)

// SubscriptionStatus enumerates known subscription states.
type SubscriptionStatus string

const (
	StatusActive    SubscriptionStatus = "active"
	StatusPastDue   SubscriptionStatus = "past_due"
	StatusCanceled  SubscriptionStatus = "canceled"
	StatusPaused    SubscriptionStatus = "paused"
	StatusTrialing  SubscriptionStatus = "trialing"
)

// Subscription is the canonical subscription record across all providers.
type Subscription struct {
	ProviderSubID      string
	ProviderCustomerID string
	PlanID             string
	Status             SubscriptionStatus
	CurrentPeriodStart time.Time
	CurrentPeriodEnd   time.Time
	CancelAtPeriodEnd  bool
	TrialEnd           *time.Time
	Metadata           map[string]string
}

// CheckoutParams are the inputs for creating a checkout session.
type CheckoutParams struct {
	UserID     string
	PlanID     string
	SuccessURL string
	CancelURL  string
	Email      string
	Metadata   map[string]string
	// RequestUUID is used as the Idempotency-Key on provider API calls.
	// Callers should set this to the X-Request-Id header value, or generate a
	// fresh UUID v4 when the header is absent. Providers that support
	// idempotency (Stripe) pass this through unchanged; others may ignore it.
	RequestUUID string
}

// CheckoutResult is returned after creating a checkout session.
type CheckoutResult struct {
	URL       string
	SessionID string
}

// PortalResult is returned after creating a billing portal URL.
type PortalResult struct {
	URL string
}

// RetryResult reports the outcome of a dunning retry attempt.
type RetryResult struct {
	Success bool
	Error   string
}

// UsageRecord is a single metered-billing entry.
type UsageRecord struct {
	SubscriptionID string
	MetricID       string
	Quantity       int
}

// Plan represents a purchasable plan from the provider.
type Plan struct {
	ID       string
	Name     string
	Price    int64  // in cents
	Currency string
	Interval string // month|year
}

// Provider is the unified interface all payment backends must implement.
type Provider interface {
	// Name returns the provider slug (stripe|lemonsqueezy|paddle).
	Name() string

	// CreateCheckout creates a hosted checkout session.
	CreateCheckout(ctx context.Context, params CheckoutParams) (*CheckoutResult, error)

	// GetSubscription fetches current subscription state by provider subscription ID.
	GetSubscription(ctx context.Context, providerSubID string) (*Subscription, error)

	// CreatePortalURL returns a billing-management portal URL for the customer.
	CreatePortalURL(ctx context.Context, providerCustomerID string, returnURL string) (*PortalResult, error)

	// CancelSubscription cancels at end of period.
	CancelSubscription(ctx context.Context, providerSubID string) error

	// PauseSubscription pauses the subscription (provider-dependent support).
	PauseSubscription(ctx context.Context, providerSubID string) error

	// RetryInvoice attempts to collect payment on a past-due invoice.
	RetryInvoice(ctx context.Context, providerSubID string) (*RetryResult, error)

	// ListPlans returns all available plans from the provider.
	ListPlans(ctx context.Context) ([]Plan, error)

	// RecordUsage records a metered-billing unit.
	RecordUsage(ctx context.Context, record UsageRecord) error

	// VerifyWebhookSignature validates the inbound webhook request.
	// Returns the raw event type string and validated body on success.
	VerifyWebhookSignature(signature string, body []byte, secret string) (eventType string, err error)

	// ParseWebhookEvent maps a raw webhook event into a canonical Subscription.
	// Returns nil subscription for non-subscription events (should be acknowledged but not synced).
	ParseWebhookEvent(eventType string, body []byte) (*Subscription, error)
}
