package internal

import (
	"encoding/json"
)

type StripeCustomer struct {
	ID              string          `json:"id"`
	Email           NullString      `json:"email"`
	Name            NullString      `json:"name"`
	Phone           NullString      `json:"phone"`
	Description     NullString      `json:"description"`
	Currency        NullString      `json:"currency"`
	DefaultSource   NullString      `json:"default_source"`
	InvoicePrefix   NullString      `json:"invoice_prefix"`
	Balance         int64           `json:"balance"`
	Delinquent      bool            `json:"delinquent"`
	TaxExempt       string          `json:"tax_exempt"`
	Metadata        json.RawMessage `json:"metadata"`
	Address         json.RawMessage `json:"address"`
	Shipping        json.RawMessage `json:"shipping"`
	SourceAccountID string          `json:"source_account_id"`
	CreatedAt       NullTime        `json:"created_at"`
	UpdatedAt       NullTime        `json:"updated_at"`
	DeletedAt       NullTime        `json:"deleted_at"`
	SyncedAt        NullTime        `json:"synced_at"`
}

// StripeProduct maps to np_stripe_products.
type StripeProduct struct {
	ID                  string          `json:"id"`
	Name                string          `json:"name"`
	Description         NullString      `json:"description"`
	Active              bool            `json:"active"`
	Type                string          `json:"type"`
	Images              json.RawMessage `json:"images"`
	Metadata            json.RawMessage `json:"metadata"`
	Attributes          json.RawMessage `json:"attributes"`
	Shippable           NullBool        `json:"shippable"`
	StatementDescriptor NullString      `json:"statement_descriptor"`
	TaxCode             NullString      `json:"tax_code"`
	UnitLabel           NullString      `json:"unit_label"`
	URL                 NullString      `json:"url"`
	DefaultPriceID      NullString      `json:"default_price_id"`
	SourceAccountID     string          `json:"source_account_id"`
	CreatedAt           NullTime        `json:"created_at"`
	UpdatedAt           NullTime        `json:"updated_at"`
	DeletedAt           NullTime        `json:"deleted_at"`
	SyncedAt            NullTime        `json:"synced_at"`
}

// StripePrice maps to np_stripe_prices.
type StripePrice struct {
	ID                string          `json:"id"`
	ProductID         NullString      `json:"product_id"`
	Active            bool            `json:"active"`
	Currency          string          `json:"currency"`
	UnitAmount        NullInt64       `json:"unit_amount"`
	UnitAmountDecimal NullString      `json:"unit_amount_decimal"`
	Type              string          `json:"type"`
	BillingScheme     string          `json:"billing_scheme"`
	Recurring         json.RawMessage `json:"recurring"`
	Tiers             json.RawMessage `json:"tiers"`
	TiersMode         NullString      `json:"tiers_mode"`
	TransformQuantity json.RawMessage `json:"transform_quantity"`
	LookupKey         NullString      `json:"lookup_key"`
	Nickname          NullString      `json:"nickname"`
	TaxBehavior       string          `json:"tax_behavior"`
	Metadata          json.RawMessage `json:"metadata"`
	SourceAccountID   string          `json:"source_account_id"`
	CreatedAt         NullTime        `json:"created_at"`
	UpdatedAt         NullTime        `json:"updated_at"`
	DeletedAt         NullTime        `json:"deleted_at"`
	SyncedAt          NullTime        `json:"synced_at"`
}

// StripeCoupon maps to np_stripe_coupons.
type StripeCoupon struct {
	ID               string          `json:"id"`
	Name             NullString      `json:"name"`
	AmountOff        NullInt64       `json:"amount_off"`
	PercentOff       NullFloat64     `json:"percent_off"`
	Currency         NullString      `json:"currency"`
	Duration         string          `json:"duration"`
	DurationInMonths NullInt32       `json:"duration_in_months"`
	MaxRedemptions   NullInt32       `json:"max_redemptions"`
	TimesRedeemed    int32           `json:"times_redeemed"`
	RedeemBy         NullTime        `json:"redeem_by"`
	Valid            bool            `json:"valid"`
	AppliesTo        json.RawMessage `json:"applies_to"`
	Metadata         json.RawMessage `json:"metadata"`
	SourceAccountID  string          `json:"source_account_id"`
	CreatedAt        NullTime        `json:"created_at"`
	UpdatedAt        NullTime        `json:"updated_at"`
	DeletedAt        NullTime        `json:"deleted_at"`
	SyncedAt         NullTime        `json:"synced_at"`
}

// StripeDiscount maps to np_stripe_discounts.
type StripeDiscount struct {
	ID                string     `json:"id"`
	CouponID          string     `json:"coupon_id"`
	CustomerID        NullString `json:"customer_id"`
	SubscriptionID    NullString `json:"subscription_id"`
	InvoiceID         NullString `json:"invoice_id"`
	InvoiceItemID     NullString `json:"invoice_item_id"`
	PromotionCodeID   NullString `json:"promotion_code_id"`
	CheckoutSessionID NullString `json:"checkout_session_id"`
	Start             NullTime   `json:"start"`
	End               NullTime   `json:"end"`
	SourceAccountID   string     `json:"source_account_id"`
}

// StripePromotionCode maps to np_stripe_promotion_codes.
type StripePromotionCode struct {
	ID              string          `json:"id"`
	CouponID        NullString      `json:"coupon_id"`
	Code            string          `json:"code"`
	CustomerID      NullString      `json:"customer_id"`
	Active          bool            `json:"active"`
	MaxRedemptions  NullInt32       `json:"max_redemptions"`
	TimesRedeemed   int32           `json:"times_redeemed"`
	ExpiresAt       NullTime        `json:"expires_at"`
	Restrictions    json.RawMessage `json:"restrictions"`
	Metadata        json.RawMessage `json:"metadata"`
	SourceAccountID string          `json:"source_account_id"`
	CreatedAt       NullTime        `json:"created_at"`
	UpdatedAt       NullTime        `json:"updated_at"`
	SyncedAt        NullTime        `json:"synced_at"`
}

// StripeSubscription maps to np_stripe_subscriptions.
type StripeSubscription struct {
	ID                     string          `json:"id"`
	CustomerID             NullString      `json:"customer_id"`
	Status                 string          `json:"status"`
	CurrentPeriodStart     NullTime        `json:"current_period_start"`
	CurrentPeriodEnd       NullTime        `json:"current_period_end"`
	CancelAt               NullTime        `json:"cancel_at"`
	CanceledAt             NullTime        `json:"canceled_at"`
	CancelAtPeriodEnd      bool            `json:"cancel_at_period_end"`
	EndedAt                NullTime        `json:"ended_at"`
	TrialStart             NullTime        `json:"trial_start"`
	TrialEnd               NullTime        `json:"trial_end"`
	CollectionMethod       string          `json:"collection_method"`
	BillingCycleAnchor     NullTime        `json:"billing_cycle_anchor"`
	BillingThresholds      json.RawMessage `json:"billing_thresholds"`
	DaysUntilDue           NullInt32       `json:"days_until_due"`
	DefaultPaymentMethodID NullString      `json:"default_payment_method_id"`
	DefaultSource          NullString      `json:"default_source"`
	Discount               json.RawMessage `json:"discount"`
	Items                  json.RawMessage `json:"items"`
	LatestInvoiceID        NullString      `json:"latest_invoice_id"`
	PendingSetupIntent     NullString      `json:"pending_setup_intent"`
	PendingUpdate          json.RawMessage `json:"pending_update"`
	ScheduleID             NullString      `json:"schedule_id"`
	StartDate              NullTime        `json:"start_date"`
	TransferData           json.RawMessage `json:"transfer_data"`
	ApplicationFeePercent  NullFloat64     `json:"application_fee_percent"`
	AutomaticTax           json.RawMessage `json:"automatic_tax"`
	PaymentSettings        json.RawMessage `json:"payment_settings"`
	Metadata               json.RawMessage `json:"metadata"`
	SourceAccountID        string          `json:"source_account_id"`
	CreatedAt              NullTime        `json:"created_at"`
	UpdatedAt              NullTime        `json:"updated_at"`
	SyncedAt               NullTime        `json:"synced_at"`
}

// StripeSubscriptionItem maps to np_stripe_subscription_items.
type StripeSubscriptionItem struct {
	ID                string          `json:"id"`
	SubscriptionID    NullString      `json:"subscription_id"`
	PriceID           NullString      `json:"price_id"`
	Quantity          int32           `json:"quantity"`
	BillingThresholds json.RawMessage `json:"billing_thresholds"`
	Metadata          json.RawMessage `json:"metadata"`
	SourceAccountID   string          `json:"source_account_id"`
	CreatedAt         NullTime        `json:"created_at"`
	SyncedAt          NullTime        `json:"synced_at"`
}

// StripeInvoice maps to np_stripe_invoices.
type StripeInvoice struct {
	ID                     string          `json:"id"`
	CustomerID             NullString      `json:"customer_id"`
	SubscriptionID         NullString      `json:"subscription_id"`
	Status                 NullString      `json:"status"`
	CollectionMethod       NullString      `json:"collection_method"`
	Currency               string          `json:"currency"`
	AmountDue              int64           `json:"amount_due"`
	AmountPaid             int64           `json:"amount_paid"`
	AmountRemaining        int64           `json:"amount_remaining"`
	Subtotal               int64           `json:"subtotal"`
	SubtotalExcludingTax   NullInt64       `json:"subtotal_excluding_tax"`
	Total                  int64           `json:"total"`
	TotalExcludingTax      NullInt64       `json:"total_excluding_tax"`
	Tax                    NullInt64       `json:"tax"`
	TotalTaxAmounts        json.RawMessage `json:"total_tax_amounts"`
	Discount               json.RawMessage `json:"discount"`
	Discounts              json.RawMessage `json:"discounts"`
	AccountCountry         NullString      `json:"account_country"`
	AccountName            NullString      `json:"account_name"`
	BillingReason          NullString      `json:"billing_reason"`
	Number                 NullString      `json:"number"`
	ReceiptNumber          NullString      `json:"receipt_number"`
	StatementDescriptor    NullString      `json:"statement_descriptor"`
	Description            NullString      `json:"description"`
	Footer                 NullString      `json:"footer"`
	CustomerEmail          NullString      `json:"customer_email"`
	CustomerName           NullString      `json:"customer_name"`
	CustomerAddress        json.RawMessage `json:"customer_address"`
	CustomerPhone          NullString      `json:"customer_phone"`
	CustomerShipping       json.RawMessage `json:"customer_shipping"`
	CustomerTaxExempt      NullString      `json:"customer_tax_exempt"`
	CustomerTaxIDs         json.RawMessage `json:"customer_tax_ids"`
	DefaultPaymentMethodID NullString      `json:"default_payment_method_id"`
	DefaultSource          NullString      `json:"default_source"`
	Lines                  json.RawMessage `json:"lines"`
	HostedInvoiceURL       NullString      `json:"hosted_invoice_url"`
	InvoicePDF             NullString      `json:"invoice_pdf"`
	PaymentIntentID        NullString      `json:"payment_intent_id"`
	ChargeID               NullString      `json:"charge_id"`
	AttemptCount           int32           `json:"attempt_count"`
	Attempted              bool            `json:"attempted"`
	AutoAdvance            NullBool        `json:"auto_advance"`
	NextPaymentAttempt     NullTime        `json:"next_payment_attempt"`
	WebhooksDeliveredAt    NullTime        `json:"webhooks_delivered_at"`
	Paid                   bool            `json:"paid"`
	PaidOutOfBand          bool            `json:"paid_out_of_band"`
	PeriodStart            NullTime        `json:"period_start"`
	PeriodEnd              NullTime        `json:"period_end"`
	DueDate                NullTime        `json:"due_date"`
	EffectiveAt            NullTime        `json:"effective_at"`
	FinalizedAt            NullTime        `json:"finalized_at"`
	MarkedUncollectibleAt  NullTime        `json:"marked_uncollectible_at"`
	VoidedAt               NullTime        `json:"voided_at"`
	Metadata               json.RawMessage `json:"metadata"`
	SourceAccountID        string          `json:"source_account_id"`
	CreatedAt              NullTime        `json:"created_at"`
	UpdatedAt              NullTime        `json:"updated_at"`
	SyncedAt               NullTime        `json:"synced_at"`
}

// StripeInvoiceItem maps to np_stripe_invoice_items.
type StripeInvoiceItem struct {
	ID                 string          `json:"id"`
	CustomerID         NullString      `json:"customer_id"`
	InvoiceID          NullString      `json:"invoice_id"`
	SubscriptionID     NullString      `json:"subscription_id"`
	SubscriptionItemID NullString      `json:"subscription_item_id"`
	PriceID            NullString      `json:"price_id"`
	Amount             int64           `json:"amount"`
	Currency           string          `json:"currency"`
	Description        NullString      `json:"description"`
	Discountable       bool            `json:"discountable"`
	Quantity           int32           `json:"quantity"`
	UnitAmount         NullInt64       `json:"unit_amount"`
	UnitAmountDecimal  NullString      `json:"unit_amount_decimal"`
	PeriodStart        NullTime        `json:"period_start"`
	PeriodEnd          NullTime        `json:"period_end"`
	Proration          bool            `json:"proration"`
	Metadata           json.RawMessage `json:"metadata"`
	SourceAccountID    string          `json:"source_account_id"`
	CreatedAt          NullTime        `json:"created_at"`
	SyncedAt           NullTime        `json:"synced_at"`
}

