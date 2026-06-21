package internal

import (
	"encoding/json"
)

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
