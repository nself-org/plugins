package internal

import (
	"encoding/json"
	"time"
)

// Transaction represents a row in np_paypal_transactions.
type Transaction struct {
	ID              string     `json:"id"`
	PayPalID        string     `json:"paypal_id"`
	Type            string     `json:"type"`
	Status          string     `json:"status"`
	Amount          float64    `json:"amount"`
	Currency        string     `json:"currency"`
	Fee             *float64   `json:"fee"`
	Net             *float64   `json:"net"`
	PayerEmail      *string    `json:"payer_email"`
	PayerName       *string    `json:"payer_name"`
	Description     *string    `json:"description"`
	CreatedAt       *time.Time `json:"created_at"`
	UpdatedAt       *time.Time `json:"updated_at"`
	SourceAccountID string     `json:"source_account_id"`
	SyncedAt        *time.Time `json:"synced_at"`
}

// Order represents a row in np_paypal_orders.
type Order struct {
	ID              string          `json:"id"`
	PayPalID        string          `json:"paypal_id"`
	Status          string          `json:"status"`
	Intent          string          `json:"intent"`
	PurchaseUnits   json.RawMessage `json:"purchase_units"`
	Payer           json.RawMessage `json:"payer"`
	CreatedAt       *time.Time      `json:"created_at"`
	UpdatedAt       *time.Time      `json:"updated_at"`
	SourceAccountID string          `json:"source_account_id"`
	SyncedAt        *time.Time      `json:"synced_at"`
}

// Capture represents a row in np_paypal_captures.
type Capture struct {
	ID              string     `json:"id"`
	PayPalID        string     `json:"paypal_id"`
	OrderID         *string    `json:"order_id"`
	Status          string     `json:"status"`
	Amount          float64    `json:"amount"`
	Currency        string     `json:"currency"`
	FinalCapture    bool       `json:"final_capture"`
	CreatedAt       *time.Time `json:"created_at"`
	UpdatedAt       *time.Time `json:"updated_at"`
	SourceAccountID string     `json:"source_account_id"`
	SyncedAt        *time.Time `json:"synced_at"`
}

// Authorization represents a row in np_paypal_authorizations.
type Authorization struct {
	ID              string     `json:"id"`
	PayPalID        string     `json:"paypal_id"`
	OrderID         *string    `json:"order_id"`
	Status          string     `json:"status"`
	Amount          float64    `json:"amount"`
	Currency        string     `json:"currency"`
	ExpirationTime  *time.Time `json:"expiration_time"`
	CreatedAt       *time.Time `json:"created_at"`
	UpdatedAt       *time.Time `json:"updated_at"`
	SourceAccountID string     `json:"source_account_id"`
	SyncedAt        *time.Time `json:"synced_at"`
}

// Refund represents a row in np_paypal_refunds.
type Refund struct {
	ID              string     `json:"id"`
	PayPalID        string     `json:"paypal_id"`
	CaptureID       *string    `json:"capture_id"`
	Status          string     `json:"status"`
	Amount          float64    `json:"amount"`
	Currency        string     `json:"currency"`
	Reason          *string    `json:"reason"`
	CreatedAt       *time.Time `json:"created_at"`
	SourceAccountID string     `json:"source_account_id"`
	SyncedAt        *time.Time `json:"synced_at"`
}

// Subscription represents a row in np_paypal_subscriptions.
type Subscription struct {
	ID              string          `json:"id"`
	PayPalID        string          `json:"paypal_id"`
	PlanID          string          `json:"plan_id"`
	Status          string          `json:"status"`
	Subscriber      json.RawMessage `json:"subscriber"`
	StartTime       *time.Time      `json:"start_time"`
	BillingInfo     json.RawMessage `json:"billing_info"`
	CreatedAt       *time.Time      `json:"created_at"`
	UpdatedAt       *time.Time      `json:"updated_at"`
	SourceAccountID string          `json:"source_account_id"`
	SyncedAt        *time.Time      `json:"synced_at"`
}

// SubscriptionPlan represents a row in np_paypal_subscription_plans.
type SubscriptionPlan struct {
	ID                 string          `json:"id"`
	PayPalID           string          `json:"paypal_id"`
	ProductID          string          `json:"product_id"`
	Name               string          `json:"name"`
	Description        *string         `json:"description"`
	Status             string          `json:"status"`
	BillingCycles      json.RawMessage `json:"billing_cycles"`
	PaymentPreferences json.RawMessage `json:"payment_preferences"`
	CreatedAt          *time.Time      `json:"created_at"`
	UpdatedAt          *time.Time      `json:"updated_at"`
	SourceAccountID    string          `json:"source_account_id"`
	SyncedAt           *time.Time      `json:"synced_at"`
}

// Product represents a row in np_paypal_products.
type Product struct {
	ID              string     `json:"id"`
	PayPalID        string     `json:"paypal_id"`
	Name            string     `json:"name"`
	Description     *string    `json:"description"`
	Type            string     `json:"type"`
	Category        *string    `json:"category"`
	ImageURL        *string    `json:"image_url"`
	HomeURL         *string    `json:"home_url"`
	CreatedAt       *time.Time `json:"created_at"`
	UpdatedAt       *time.Time `json:"updated_at"`
	SourceAccountID string     `json:"source_account_id"`
	SyncedAt        *time.Time `json:"synced_at"`
}

// Dispute represents a row in np_paypal_disputes.
type Dispute struct {
	ID              string          `json:"id"`
	PayPalID        string          `json:"paypal_id"`
	Reason          string          `json:"reason"`
	Status          string          `json:"status"`
	DisputeAmount   float64         `json:"dispute_amount"`
	DisputeCurrency string          `json:"dispute_currency"`
	Messages        json.RawMessage `json:"messages"`
	CreatedAt       *time.Time      `json:"created_at"`
	UpdatedAt       *time.Time      `json:"updated_at"`
	SourceAccountID string          `json:"source_account_id"`
	SyncedAt        *time.Time      `json:"synced_at"`
}

// Payout represents a row in np_paypal_payouts.
type Payout struct {
	ID              string     `json:"id"`
	PayPalID        string     `json:"paypal_id"`
	BatchID         string     `json:"batch_id"`
	Status          string     `json:"status"`
	Amount          *float64   `json:"amount"`
	Currency        *string    `json:"currency"`
	RecipientType   *string    `json:"recipient_type"`
	Receiver        *string    `json:"receiver"`
	SenderItemID    *string    `json:"sender_item_id"`
	CreatedAt       *time.Time `json:"created_at"`
	SourceAccountID string     `json:"source_account_id"`
	SyncedAt        *time.Time `json:"synced_at"`
}

// Invoice represents a row in np_paypal_invoices.
type Invoice struct {
	ID              string          `json:"id"`
	PayPalID        string          `json:"paypal_id"`
	Status          string          `json:"status"`
	Detail          json.RawMessage `json:"detail"`
	Amount          *float64        `json:"amount"`
	Currency        *string         `json:"currency"`
	DueDate         *string         `json:"due_date"`
	Invoicer        json.RawMessage `json:"invoicer"`
	CreatedAt       *time.Time      `json:"created_at"`
	UpdatedAt       *time.Time      `json:"updated_at"`
	SourceAccountID string          `json:"source_account_id"`
	SyncedAt        *time.Time      `json:"synced_at"`
}

// Payer represents a row in np_paypal_payers.
type Payer struct {
	ID              string          `json:"id"`
	PayPalID        string          `json:"paypal_id"`
	Email           *string         `json:"email"`
	Name            *string         `json:"name"`
	Phone           *string         `json:"phone"`
	Address         json.RawMessage `json:"address"`
	SourceAccountID string          `json:"source_account_id"`
	SyncedAt        *time.Time      `json:"synced_at"`
}

// Balance represents a row in np_paypal_balances.
type Balance struct {
	ID               string     `json:"id"`
	Currency         string     `json:"currency"`
	TotalBalance     *float64   `json:"total_balance"`
	AvailableBalance *float64   `json:"available_balance"`
	WithheldBalance  *float64   `json:"withheld_balance"`
	RecordedAt       *time.Time `json:"recorded_at"`
	SourceAccountID  string     `json:"source_account_id"`
}

// WebhookEvent represents a row in np_paypal_webhook_events.
type WebhookEvent struct {
	ID              string          `json:"id"`
	PayPalEventID   string          `json:"paypal_event_id"`
	EventType       string          `json:"event_type"`
	ResourceType    string          `json:"resource_type"`
	Resource        json.RawMessage `json:"resource"`
	Summary         *string         `json:"summary"`
	CreateTime      *time.Time      `json:"create_time"`
	Processed       bool            `json:"processed"`
	SourceAccountID string          `json:"source_account_id"`
}

// SyncResult holds the outcome of a sync operation.
type SyncResult struct {
	Success  bool              `json:"success"`
	Synced   map[string]int    `json:"synced"`
	Errors   []string          `json:"errors"`
	Duration string            `json:"duration"`
}

// SyncStats holds per-table record counts.
type SyncStats struct {
	Transactions      int        `json:"transactions"`
	Orders            int        `json:"orders"`
	Captures          int        `json:"captures"`
	Authorizations    int        `json:"authorizations"`
	Refunds           int        `json:"refunds"`
	Subscriptions     int        `json:"subscriptions"`
	SubscriptionPlans int        `json:"subscription_plans"`
	Products          int        `json:"products"`
	Disputes          int        `json:"disputes"`
	Payouts           int        `json:"payouts"`
	Invoices          int        `json:"invoices"`
	Payers            int        `json:"payers"`
	Balances          int        `json:"balances"`
	WebhookEvents     int        `json:"webhook_events"`
	LastSyncedAt      *time.Time `json:"last_synced_at"`
}
