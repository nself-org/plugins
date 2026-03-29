package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HandleWebhook processes incoming PayPal webhook events.
// It validates the request structure, stores the raw event, and routes
// to the appropriate handler based on event_type.
func HandleWebhook(pool *pgxpool.Pool, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		// Read the raw body.
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// Basic structural validation: verify the Transmission-Id header exists
		// and the body parses as valid JSON with required fields.
		transmissionID := r.Header.Get("Paypal-Transmission-Id")
		if transmissionID == "" {
			log.Printf("[nself-paypal] webhook: missing Paypal-Transmission-Id header")
			http.Error(w, "missing transmission id", http.StatusBadRequest)
			return
		}

		var event webhookPayload
		if err := json.Unmarshal(body, &event); err != nil {
			log.Printf("[nself-paypal] webhook: invalid JSON: %v", err)
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		if event.ID == "" || event.EventType == "" {
			log.Printf("[nself-paypal] webhook: missing id or event_type")
			http.Error(w, "missing required fields", http.StatusBadRequest)
			return
		}

		// Store the raw webhook event.
		summary := nilIfEmpty(event.Summary)
		createTime := parseTimePtr(event.CreateTime)

		resource := event.Resource
		if len(resource) == 0 {
			resource = json.RawMessage("{}")
		}

		err = InsertWebhookEvent(ctx, pool, &WebhookEvent{
			PayPalEventID:   event.ID,
			EventType:       event.EventType,
			ResourceType:    event.ResourceType,
			Resource:        resource,
			Summary:         summary,
			CreateTime:      createTime,
			Processed:       false,
			SourceAccountID: "primary",
		})
		if err != nil {
			log.Printf("[nself-paypal] webhook: failed to store event %s: %v", event.ID, err)
		}

		// Route to the appropriate handler.
		processErr := routeEvent(ctx, pool, &event)
		if processErr != nil {
			log.Printf("[nself-paypal] webhook: error processing %s (%s): %v", event.EventType, event.ID, processErr)
			// Still return 200 to prevent PayPal from retrying on processing errors.
		} else {
			log.Printf("[nself-paypal] webhook: processed %s (%s)", event.EventType, event.ID)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"received"}`))
	}
}

// webhookPayload represents the incoming PayPal webhook event structure.
type webhookPayload struct {
	ID           string          `json:"id"`
	EventType    string          `json:"event_type"`
	ResourceType string          `json:"resource_type"`
	Resource     json.RawMessage `json:"resource"`
	Summary      string          `json:"summary"`
	CreateTime   string          `json:"create_time"`
}

// routeEvent dispatches webhook events to the correct handler based on event_type prefix.
func routeEvent(ctx context.Context, pool *pgxpool.Pool, event *webhookPayload) error {
	switch event.EventType {
	// Payment Captures
	case "PAYMENT.CAPTURE.COMPLETED",
		"PAYMENT.CAPTURE.DENIED",
		"PAYMENT.CAPTURE.REFUNDED",
		"PAYMENT.CAPTURE.REVERSED",
		"PAYMENT.CAPTURE.PENDING":
		return handleCaptureEvent(ctx, pool, event)

	// Checkout Orders
	case "CHECKOUT.ORDER.COMPLETED",
		"CHECKOUT.ORDER.APPROVED",
		"CHECKOUT.ORDER.VOIDED":
		return handleOrderEvent(ctx, pool, event)

	// Subscriptions
	case "BILLING.SUBSCRIPTION.CREATED",
		"BILLING.SUBSCRIPTION.ACTIVATED",
		"BILLING.SUBSCRIPTION.UPDATED",
		"BILLING.SUBSCRIPTION.CANCELLED",
		"BILLING.SUBSCRIPTION.SUSPENDED",
		"BILLING.SUBSCRIPTION.EXPIRED":
		return handleSubscriptionEvent(ctx, pool, event)

	// Disputes
	case "CUSTOMER.DISPUTE.CREATED",
		"CUSTOMER.DISPUTE.UPDATED",
		"CUSTOMER.DISPUTE.RESOLVED",
		"CUSTOMER.DISPUTE.OTHER":
		return handleDisputeEvent(ctx, pool, event)

	// Payouts
	case "PAYMENT.PAYOUTSBATCH.SUCCESS",
		"PAYMENT.PAYOUTSBATCH.DENIED",
		"PAYMENT.PAYOUTSBATCH.PROCESSING":
		return handlePayoutEvent(ctx, pool, event)

	// Invoices
	case "INVOICING.INVOICE.PAID",
		"INVOICING.INVOICE.CANCELLED":
		return handleInvoiceEvent(ctx, pool, event)

	// Sales (legacy)
	case "PAYMENT.SALE.REFUNDED",
		"PAYMENT.SALE.COMPLETED":
		return handleSaleEvent(ctx, pool, event)

	default:
		log.Printf("[nself-paypal] webhook: unhandled event type: %s", event.EventType)
		return nil
	}
}

// --- Event Handlers ----------------------------------------------------------

func handleCaptureEvent(ctx context.Context, pool *pgxpool.Pool, event *webhookPayload) error {
	var resource struct {
		ID           string `json:"id"`
		Status       string `json:"status"`
		Amount       *Money `json:"amount"`
		FinalCapture bool   `json:"final_capture"`
		CreateTime   string `json:"create_time"`
		UpdateTime   string `json:"update_time"`
	}
	if err := json.Unmarshal(event.Resource, &resource); err != nil {
		return fmt.Errorf("unmarshal capture resource: %w", err)
	}

	amount := float64(0)
	currency := "USD"
	if resource.Amount != nil {
		amount = parseFloat(resource.Amount.Value)
		currency = resource.Amount.CurrencyCode
	}

	return UpsertCapture(ctx, pool, &Capture{
		PayPalID:        resource.ID,
		Status:          resource.Status,
		Amount:          amount,
		Currency:        currency,
		FinalCapture:    resource.FinalCapture,
		CreatedAt:       parseTimePtr(resource.CreateTime),
		UpdatedAt:       parseTimePtr(resource.UpdateTime),
		SourceAccountID: "primary",
	})
}

func handleOrderEvent(ctx context.Context, pool *pgxpool.Pool, event *webhookPayload) error {
	var resource struct {
		ID            string          `json:"id"`
		Status        string          `json:"status"`
		Intent        string          `json:"intent"`
		PurchaseUnits json.RawMessage `json:"purchase_units"`
		Payer         json.RawMessage `json:"payer"`
		CreateTime    string          `json:"create_time"`
		UpdateTime    string          `json:"update_time"`
	}
	if err := json.Unmarshal(event.Resource, &resource); err != nil {
		return fmt.Errorf("unmarshal order resource: %w", err)
	}

	pu := resource.PurchaseUnits
	if len(pu) == 0 {
		pu = json.RawMessage("[]")
	}
	payer := resource.Payer
	if len(payer) == 0 {
		payer = json.RawMessage("{}")
	}

	return UpsertOrder(ctx, pool, &Order{
		PayPalID:        resource.ID,
		Status:          resource.Status,
		Intent:          resource.Intent,
		PurchaseUnits:   pu,
		Payer:           payer,
		CreatedAt:       parseTimePtr(resource.CreateTime),
		UpdatedAt:       parseTimePtr(resource.UpdateTime),
		SourceAccountID: "primary",
	})
}

func handleSubscriptionEvent(ctx context.Context, pool *pgxpool.Pool, event *webhookPayload) error {
	var resource struct {
		ID          string          `json:"id"`
		PlanID      string          `json:"plan_id"`
		Status      string          `json:"status"`
		Subscriber  json.RawMessage `json:"subscriber"`
		StartTime   string          `json:"start_time"`
		BillingInfo json.RawMessage `json:"billing_info"`
		CreateTime  string          `json:"create_time"`
		UpdateTime  string          `json:"update_time"`
	}
	if err := json.Unmarshal(event.Resource, &resource); err != nil {
		return fmt.Errorf("unmarshal subscription resource: %w", err)
	}

	sub := resource.Subscriber
	if len(sub) == 0 {
		sub = json.RawMessage("{}")
	}
	bi := resource.BillingInfo
	if len(bi) == 0 {
		bi = json.RawMessage("{}")
	}

	return UpsertSubscription(ctx, pool, &Subscription{
		PayPalID:        resource.ID,
		PlanID:          resource.PlanID,
		Status:          resource.Status,
		Subscriber:      sub,
		StartTime:       parseTimePtr(resource.StartTime),
		BillingInfo:     bi,
		CreatedAt:       parseTimePtr(resource.CreateTime),
		UpdatedAt:       parseTimePtr(resource.UpdateTime),
		SourceAccountID: "primary",
	})
}

func handleDisputeEvent(ctx context.Context, pool *pgxpool.Pool, event *webhookPayload) error {
	var resource struct {
		DisputeID     string          `json:"dispute_id"`
		Reason        string          `json:"reason"`
		Status        string          `json:"status"`
		DisputeAmount *Money          `json:"dispute_amount"`
		Messages      json.RawMessage `json:"messages"`
		CreateTime    string          `json:"create_time"`
		UpdateTime    string          `json:"update_time"`
	}
	if err := json.Unmarshal(event.Resource, &resource); err != nil {
		return fmt.Errorf("unmarshal dispute resource: %w", err)
	}

	amount := float64(0)
	currency := "USD"
	if resource.DisputeAmount != nil {
		amount = parseFloat(resource.DisputeAmount.Value)
		currency = resource.DisputeAmount.CurrencyCode
	}

	msgs := resource.Messages
	if len(msgs) == 0 {
		msgs = json.RawMessage("[]")
	}

	return UpsertDispute(ctx, pool, &Dispute{
		PayPalID:        resource.DisputeID,
		Reason:          resource.Reason,
		Status:          resource.Status,
		DisputeAmount:   amount,
		DisputeCurrency: currency,
		Messages:        msgs,
		CreatedAt:       parseTimePtr(resource.CreateTime),
		UpdatedAt:       parseTimePtr(resource.UpdateTime),
		SourceAccountID: "primary",
	})
}

func handlePayoutEvent(ctx context.Context, pool *pgxpool.Pool, event *webhookPayload) error {
	var resource struct {
		BatchHeader struct {
			PayoutBatchID     string `json:"payout_batch_id"`
			BatchStatus       string `json:"batch_status"`
			TimeCreated       string `json:"time_created"`
			SenderBatchHeader struct {
				SenderBatchID string `json:"sender_batch_id"`
			} `json:"sender_batch_header"`
			Amount *Money `json:"amount"`
		} `json:"batch_header"`
	}
	if err := json.Unmarshal(event.Resource, &resource); err != nil {
		return fmt.Errorf("unmarshal payout resource: %w", err)
	}

	header := resource.BatchHeader
	var amount *float64
	var currency *string
	if header.Amount != nil {
		a := parseFloat(header.Amount.Value)
		amount = &a
		currency = &header.Amount.CurrencyCode
	}

	senderBatchID := nilIfEmpty(header.SenderBatchHeader.SenderBatchID)

	return UpsertPayout(ctx, pool, &Payout{
		PayPalID:        header.PayoutBatchID,
		BatchID:         header.PayoutBatchID,
		Status:          header.BatchStatus,
		Amount:          amount,
		Currency:        currency,
		SenderItemID:    senderBatchID,
		CreatedAt:       parseTimePtr(header.TimeCreated),
		SourceAccountID: "primary",
	})
}

func handleInvoiceEvent(ctx context.Context, pool *pgxpool.Pool, event *webhookPayload) error {
	var resource struct {
		ID         string          `json:"id"`
		Status     string          `json:"status"`
		Detail     json.RawMessage `json:"detail"`
		Amount     *Money          `json:"amount"`
		Invoicer   json.RawMessage `json:"invoicer"`
		CreateTime string          `json:"create_time"`
		UpdateTime string          `json:"update_time"`
	}
	if err := json.Unmarshal(event.Resource, &resource); err != nil {
		return fmt.Errorf("unmarshal invoice resource: %w", err)
	}

	var amount *float64
	var currency *string
	if resource.Amount != nil {
		a := parseFloat(resource.Amount.Value)
		amount = &a
		currency = &resource.Amount.CurrencyCode
	}

	detail := resource.Detail
	if len(detail) == 0 {
		detail = json.RawMessage("{}")
	}
	invoicer := resource.Invoicer
	if len(invoicer) == 0 {
		invoicer = json.RawMessage("{}")
	}

	return UpsertInvoice(ctx, pool, &Invoice{
		PayPalID:        resource.ID,
		Status:          resource.Status,
		Detail:          detail,
		Amount:          amount,
		Currency:        currency,
		Invoicer:        invoicer,
		CreatedAt:       parseTimePtr(resource.CreateTime),
		UpdatedAt:       parseTimePtr(resource.UpdateTime),
		SourceAccountID: "primary",
	})
}

func handleSaleEvent(ctx context.Context, pool *pgxpool.Pool, event *webhookPayload) error {
	var resource struct {
		ID         string `json:"id"`
		State      string `json:"state"`
		Amount     *Money `json:"amount"`
		CreateTime string `json:"create_time"`
		UpdateTime string `json:"update_time"`
	}
	if err := json.Unmarshal(event.Resource, &resource); err != nil {
		return fmt.Errorf("unmarshal sale resource: %w", err)
	}

	amount := float64(0)
	currency := "USD"
	if resource.Amount != nil {
		amount = parseFloat(resource.Amount.Value)
		currency = resource.Amount.CurrencyCode
	}

	return UpsertTransaction(ctx, pool, &Transaction{
		PayPalID:        resource.ID,
		Type:            "sale",
		Status:          resource.State,
		Amount:          amount,
		Currency:        currency,
		CreatedAt:       parseTimePtr(resource.CreateTime),
		UpdatedAt:       parseTimePtr(resource.UpdateTime),
		SourceAccountID: "primary",
	})
}
