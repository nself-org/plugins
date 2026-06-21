package internal

import (
	"encoding/json"
)

type StripePayout struct {
	ID              string          `json:"id"`
	Amount          int64           `json:"amount"`
	Currency        string          `json:"currency"`
	Status          string          `json:"status"`
	Type            string          `json:"type"`
	Method          NullString      `json:"method"`
	Description     NullString      `json:"description"`
	ArrivalDate     NullTime        `json:"arrival_date"`
	Metadata        json.RawMessage `json:"metadata"`
	SourceAccountID string          `json:"source_account_id"`
	CreatedAt       NullTime        `json:"created_at"`
	SyncedAt        NullTime        `json:"synced_at"`
}

// StripeTaxRate maps to np_stripe_tax_rates.
type StripeTaxRate struct {
	ID              string          `json:"id"`
	DisplayName     string          `json:"display_name"`
	Description     NullString      `json:"description"`
	Percentage      float64         `json:"percentage"`
	Inclusive       bool            `json:"inclusive"`
	Active          bool            `json:"active"`
	Country         NullString      `json:"country"`
	State           NullString      `json:"state"`
	Jurisdiction    NullString      `json:"jurisdiction"`
	TaxType         NullString      `json:"tax_type"`
	Metadata        json.RawMessage `json:"metadata"`
	SourceAccountID string          `json:"source_account_id"`
	CreatedAt       NullTime        `json:"created_at"`
	SyncedAt        NullTime        `json:"synced_at"`
}

// StripeTaxID maps to np_stripe_tax_ids.
type StripeTaxID struct {
	ID              string          `json:"id"`
	CustomerID      NullString      `json:"customer_id"`
	Type            string          `json:"type"`
	Value           string          `json:"value"`
	Country         NullString      `json:"country"`
	Verification    json.RawMessage `json:"verification"`
	SourceAccountID string          `json:"source_account_id"`
	CreatedAt       NullTime        `json:"created_at"`
	SyncedAt        NullTime        `json:"synced_at"`
}

// StripeCheckoutSession maps to np_stripe_checkout_sessions.
type StripeCheckoutSession struct {
	ID                        string          `json:"id"`
	CustomerID                NullString      `json:"customer_id"`
	CustomerEmail             NullString      `json:"customer_email"`
	PaymentIntentID           NullString      `json:"payment_intent_id"`
	SubscriptionID            NullString      `json:"subscription_id"`
	InvoiceID                 NullString      `json:"invoice_id"`
	Mode                      string          `json:"mode"`
	Status                    NullString      `json:"status"`
	PaymentStatus             NullString      `json:"payment_status"`
	Currency                  NullString      `json:"currency"`
	AmountTotal               NullInt64       `json:"amount_total"`
	AmountSubtotal            NullInt64       `json:"amount_subtotal"`
	TotalDetails              json.RawMessage `json:"total_details"`
	SuccessURL                NullString      `json:"success_url"`
	CancelURL                 NullString      `json:"cancel_url"`
	URL                       NullString      `json:"url"`
	ClientReferenceID         NullString      `json:"client_reference_id"`
	CustomerCreation          NullString      `json:"customer_creation"`
	BillingAddressCollection  NullString      `json:"billing_address_collection"`
	ShippingAddressCollection json.RawMessage `json:"shipping_address_collection"`
	ShippingCost              json.RawMessage `json:"shipping_cost"`
	ShippingDetails           json.RawMessage `json:"shipping_details"`
	CustomText                json.RawMessage `json:"custom_text"`
	Consent                   json.RawMessage `json:"consent"`
	ConsentCollection         json.RawMessage `json:"consent_collection"`
	ExpiresAt                 NullTime        `json:"expires_at"`
	Livemode                  bool            `json:"livemode"`
	Locale                    NullString      `json:"locale"`
	Metadata                  json.RawMessage `json:"metadata"`
	SourceAccountID           string          `json:"source_account_id"`
	CreatedAt                 NullTime        `json:"created_at"`
	SyncedAt                  NullTime        `json:"synced_at"`
}

// StripeSetupIntent maps to np_stripe_setup_intents.
type StripeSetupIntent struct {
	ID                 string          `json:"id"`
	CustomerID         NullString      `json:"customer_id"`
	PaymentMethodID    NullString      `json:"payment_method_id"`
	Status             string          `json:"status"`
	Usage              string          `json:"usage"`
	PaymentMethodTypes json.RawMessage `json:"payment_method_types"`
	ClientSecret       NullString      `json:"client_secret"`
	Description        NullString      `json:"description"`
	CancellationReason NullString      `json:"cancellation_reason"`
	LastSetupError     json.RawMessage `json:"last_setup_error"`
	NextAction         json.RawMessage `json:"next_action"`
	SingleUseMandate   NullString      `json:"single_use_mandate"`
	Mandate            NullString      `json:"mandate"`
	OnBehalfOf         NullString      `json:"on_behalf_of"`
	Application        NullString      `json:"application"`
	Metadata           json.RawMessage `json:"metadata"`
	SourceAccountID    string          `json:"source_account_id"`
	CreatedAt          NullTime        `json:"created_at"`
	UpdatedAt          NullTime        `json:"updated_at"`
	SyncedAt           NullTime        `json:"synced_at"`
}

// StripeWebhookEvent maps to np_stripe_webhook_events.
type StripeWebhookEvent struct {
	ID                    string          `json:"id"`
	Type                  string          `json:"type"`
	APIVersion            NullString      `json:"api_version"`
	Data                  json.RawMessage `json:"data"`
	ObjectType            NullString      `json:"object_type"`
	ObjectID              NullString      `json:"object_id"`
	RequestID             NullString      `json:"request_id"`
	RequestIdempotencyKey NullString      `json:"request_idempotency_key"`
	Livemode              bool            `json:"livemode"`
	PendingWebhooks       int32           `json:"pending_webhooks"`
	Processed             bool            `json:"processed"`
	ProcessedAt           NullTime        `json:"processed_at"`
	Error                 NullString      `json:"error"`
	RetryCount            int32           `json:"retry_count"`
	SourceAccountID       string          `json:"source_account_id"`
	CreatedAt             NullTime        `json:"created_at"`
	ReceivedAt            NullTime        `json:"received_at"`
}

// SyncStats holds aggregate counts for the /api/stats endpoint.
type SyncStats struct {
	Customers           int64    `json:"customers"`
	Products            int64    `json:"products"`
	Prices              int64    `json:"prices"`
	Coupons             int64    `json:"coupons"`
	PromotionCodes      int64    `json:"promotionCodes"`
	Subscriptions       int64    `json:"subscriptions"`
	SubscriptionItems   int64    `json:"subscriptionItems"`
	Invoices            int64    `json:"invoices"`
	InvoiceItems        int64    `json:"invoiceItems"`
	Charges             int64    `json:"charges"`
	Refunds             int64    `json:"refunds"`
	Disputes            int64    `json:"disputes"`
	PaymentIntents      int64    `json:"paymentIntents"`
	SetupIntents        int64    `json:"setupIntents"`
	PaymentMethods      int64    `json:"paymentMethods"`
	BalanceTransactions int64    `json:"balanceTransactions"`
	CheckoutSessions    int64    `json:"checkoutSessions"`
	TaxIDs              int64    `json:"taxIds"`
	TaxRates            int64    `json:"taxRates"`
	LastSyncedAt        *string  `json:"lastSyncedAt"`
}

// ListResponse is the standard paginated response envelope.
type ListResponse struct {
	Data   interface{} `json:"data"`
	Total  int64       `json:"total"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}
