package internal

import (
	"context"
	"log"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
