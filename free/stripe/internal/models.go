package internal

import (
	"encoding/json"
	"time"
)

// NullTime represents a nullable timestamp for JSON serialization.
type NullTime struct {
	Time  time.Time
	Valid bool
}

func (nt NullTime) MarshalJSON() ([]byte, error) {
	if !nt.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(nt.Time)
}

// NullString represents a nullable string for JSON serialization.
type NullString struct {
	String string
	Valid  bool
}

func (ns NullString) MarshalJSON() ([]byte, error) {
	if !ns.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(ns.String)
}

// NullInt64 represents a nullable int64 for JSON serialization.
type NullInt64 struct {
	Int64 int64
	Valid bool
}

func (ni NullInt64) MarshalJSON() ([]byte, error) {
	if !ni.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(ni.Int64)
}

// NullBool represents a nullable bool for JSON serialization.
type NullBool struct {
	Bool  bool
	Valid bool
}

func (nb NullBool) MarshalJSON() ([]byte, error) {
	if !nb.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(nb.Bool)
}

// NullFloat64 represents a nullable float64 for JSON serialization.
type NullFloat64 struct {
	Float64 float64
	Valid   bool
}

func (nf NullFloat64) MarshalJSON() ([]byte, error) {
	if !nf.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(nf.Float64)
}

// NullInt32 represents a nullable int32 for JSON serialization.
type NullInt32 struct {
	Int32 int32
	Valid bool
}

func (ni NullInt32) MarshalJSON() ([]byte, error) {
	if !ni.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(ni.Int32)
}

// StripeCustomer maps to np_stripe_customers.
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

// StripePaymentIntent maps to np_stripe_payment_intents.
type StripePaymentIntent struct {
	ID                      string          `json:"id"`
	CustomerID              NullString      `json:"customer_id"`
	InvoiceID               NullString      `json:"invoice_id"`
	Amount                  int64           `json:"amount"`
	AmountCapturable        int64           `json:"amount_capturable"`
	AmountReceived          int64           `json:"amount_received"`
	Currency                string          `json:"currency"`
	Status                  string          `json:"status"`
	CaptureMethod           string          `json:"capture_method"`
	ConfirmationMethod      string          `json:"confirmation_method"`
	PaymentMethodID         NullString      `json:"payment_method_id"`
	PaymentMethodTypes      json.RawMessage `json:"payment_method_types"`
	SetupFutureUsage        NullString      `json:"setup_future_usage"`
	ClientSecret            NullString      `json:"client_secret"`
	Description             NullString      `json:"description"`
	ReceiptEmail            NullString      `json:"receipt_email"`
	StatementDescriptor     NullString      `json:"statement_descriptor"`
	StatementDescriptorSuff NullString      `json:"statement_descriptor_suffix"`
	Shipping                json.RawMessage `json:"shipping"`
	ApplicationFeeAmount    NullInt64       `json:"application_fee_amount"`
	TransferData            json.RawMessage `json:"transfer_data"`
	TransferGroup           NullString      `json:"transfer_group"`
	OnBehalfOf              NullString      `json:"on_behalf_of"`
	CancellationReason      NullString      `json:"cancellation_reason"`
	CanceledAt              NullTime        `json:"canceled_at"`
	Charges                 json.RawMessage `json:"charges"`
	LastPaymentError        json.RawMessage `json:"last_payment_error"`
	NextAction              json.RawMessage `json:"next_action"`
	Processing              json.RawMessage `json:"processing"`
	Review                  NullString      `json:"review"`
	AutomaticPaymentMethods json.RawMessage `json:"automatic_payment_methods"`
	Metadata                json.RawMessage `json:"metadata"`
	SourceAccountID         string          `json:"source_account_id"`
	CreatedAt               NullTime        `json:"created_at"`
	UpdatedAt               NullTime        `json:"updated_at"`
	SyncedAt                NullTime        `json:"synced_at"`
}

// StripePaymentMethod maps to np_stripe_payment_methods.
type StripePaymentMethod struct {
	ID              string          `json:"id"`
	CustomerID      NullString      `json:"customer_id"`
	Type            string          `json:"type"`
	BillingDetails  json.RawMessage `json:"billing_details"`
	Card            json.RawMessage `json:"card"`
	BankAccount     json.RawMessage `json:"bank_account"`
	SepaDebit       json.RawMessage `json:"sepa_debit"`
	USBankAccount   json.RawMessage `json:"us_bank_account"`
	Link            json.RawMessage `json:"link"`
	Metadata        json.RawMessage `json:"metadata"`
	SourceAccountID string          `json:"source_account_id"`
	CreatedAt       NullTime        `json:"created_at"`
	UpdatedAt       NullTime        `json:"updated_at"`
	SyncedAt        NullTime        `json:"synced_at"`
}

// StripeCharge maps to np_stripe_charges.
type StripeCharge struct {
	ID                        string          `json:"id"`
	CustomerID                NullString      `json:"customer_id"`
	PaymentIntentID           NullString      `json:"payment_intent_id"`
	InvoiceID                 NullString      `json:"invoice_id"`
	Amount                    int64           `json:"amount"`
	AmountCaptured            int64           `json:"amount_captured"`
	AmountRefunded            int64           `json:"amount_refunded"`
	Currency                  string          `json:"currency"`
	Status                    string          `json:"status"`
	Paid                      bool            `json:"paid"`
	Captured                  bool            `json:"captured"`
	Refunded                  bool            `json:"refunded"`
	Disputed                  bool            `json:"disputed"`
	FailureCode               NullString      `json:"failure_code"`
	FailureMessage            NullString      `json:"failure_message"`
	Outcome                   json.RawMessage `json:"outcome"`
	Description               NullString      `json:"description"`
	ReceiptEmail              NullString      `json:"receipt_email"`
	ReceiptNumber             NullString      `json:"receipt_number"`
	ReceiptURL                NullString      `json:"receipt_url"`
	StatementDescriptor       NullString      `json:"statement_descriptor"`
	StatementDescriptorSuffix NullString      `json:"statement_descriptor_suffix"`
	PaymentMethodID           NullString      `json:"payment_method_id"`
	PaymentMethodDetails      json.RawMessage `json:"payment_method_details"`
	BillingDetails            json.RawMessage `json:"billing_details"`
	ShippingDetails           json.RawMessage `json:"shipping"`
	FraudDetails              json.RawMessage `json:"fraud_details"`
	BalanceTransactionID      NullString      `json:"balance_transaction_id"`
	ApplicationFeeID          NullString      `json:"application_fee_id"`
	ApplicationFeeAmount      NullInt64       `json:"application_fee_amount"`
	TransferID                NullString      `json:"transfer_id"`
	TransferGroup             NullString      `json:"transfer_group"`
	OnBehalfOf                NullString      `json:"on_behalf_of"`
	SourceTransfer            NullString      `json:"source_transfer"`
	Metadata                  json.RawMessage `json:"metadata"`
	SourceAccountID           string          `json:"source_account_id"`
	CreatedAt                 NullTime        `json:"created_at"`
	UpdatedAt                 NullTime        `json:"updated_at"`
	SyncedAt                  NullTime        `json:"synced_at"`
}

// StripeRefund maps to np_stripe_refunds.
type StripeRefund struct {
	ID                        string          `json:"id"`
	ChargeID                  NullString      `json:"charge_id"`
	PaymentIntentID           NullString      `json:"payment_intent_id"`
	Amount                    int64           `json:"amount"`
	Currency                  string          `json:"currency"`
	Status                    string          `json:"status"`
	Reason                    NullString      `json:"reason"`
	ReceiptNumber             NullString      `json:"receipt_number"`
	Description               NullString      `json:"description"`
	FailureBalanceTransaction NullString      `json:"failure_balance_transaction"`
	FailureReason             NullString      `json:"failure_reason"`
	BalanceTransactionID      NullString      `json:"balance_transaction_id"`
	SourceTransferReversal    NullString      `json:"source_transfer_reversal"`
	TransferReversal          NullString      `json:"transfer_reversal"`
	Metadata                  json.RawMessage `json:"metadata"`
	SourceAccountID           string          `json:"source_account_id"`
	CreatedAt                 NullTime        `json:"created_at"`
	SyncedAt                  NullTime        `json:"synced_at"`
}

// StripeDispute maps to np_stripe_disputes.
type StripeDispute struct {
	ID                  string          `json:"id"`
	ChargeID            NullString      `json:"charge_id"`
	PaymentIntentID     NullString      `json:"payment_intent_id"`
	Amount              int64           `json:"amount"`
	Currency            string          `json:"currency"`
	Status              string          `json:"status"`
	Reason              string          `json:"reason"`
	IsChargeRefundable  bool            `json:"is_charge_refundable"`
	BalanceTransactions json.RawMessage `json:"balance_transactions"`
	Evidence            json.RawMessage `json:"evidence"`
	EvidenceDetails     json.RawMessage `json:"evidence_details"`
	Metadata            json.RawMessage `json:"metadata"`
	SourceAccountID     string          `json:"source_account_id"`
	CreatedAt           NullTime        `json:"created_at"`
	UpdatedAt           NullTime        `json:"updated_at"`
	SyncedAt            NullTime        `json:"synced_at"`
}

// StripeBalanceTransaction maps to np_stripe_balance_transactions.
type StripeBalanceTransaction struct {
	ID                string          `json:"id"`
	Amount            int64           `json:"amount"`
	Currency          string          `json:"currency"`
	Net               int64           `json:"net"`
	Fee               int64           `json:"fee"`
	FeeDetails        json.RawMessage `json:"fee_details"`
	Type              string          `json:"type"`
	Status            string          `json:"status"`
	Description       NullString      `json:"description"`
	Source            NullString      `json:"source"`
	ReportingCategory NullString      `json:"reporting_category"`
	AvailableOn       NullTime        `json:"available_on"`
	SourceAccountID   string          `json:"source_account_id"`
	CreatedAt         NullTime        `json:"created_at"`
	SyncedAt          NullTime        `json:"synced_at"`
}

// StripePayout maps to np_stripe_payouts.
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
