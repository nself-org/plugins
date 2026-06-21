package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
