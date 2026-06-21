package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
