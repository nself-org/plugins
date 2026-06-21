package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
