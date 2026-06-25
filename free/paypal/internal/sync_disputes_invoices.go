package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"github.com/jackc/pgx/v5/pgxpool"
)

func syncDisputes(ctx context.Context, pool *pgxpool.Pool, client *PayPalClient, accountID string) (int, error) {
	disputes, err := client.ListDisputes()
	if err != nil {
		return 0, err
	}

	count := 0
	for _, d := range disputes {
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		createdAt := parseTimePtr(d.CreateTime)
		updatedAt := parseTimePtr(d.UpdateTime)

		amount := float64(0)
		currency := "USD"
		if d.DisputeAmount != nil {
			amount = parseFloat(d.DisputeAmount.Value)
			currency = d.DisputeAmount.CurrencyCode
		}

		msgs := d.Messages
		if len(msgs) == 0 {
			msgs = json.RawMessage("[]")
		}

		err := UpsertDispute(dbCtx, pool, &Dispute{
			PayPalID:        d.DisputeID,
			Reason:          d.Reason,
			Status:          d.Status,
			DisputeAmount:   amount,
			DisputeCurrency: currency,
			Messages:        msgs,
			CreatedAt:       createdAt,
			UpdatedAt:       updatedAt,
			SourceAccountID: accountID,
		})
		cancel()
		if err != nil {
			return count, fmt.Errorf("upsert dispute %s: %w", d.DisputeID, err)
		}
		count++
	}
	return count, nil
}

// syncInvoices fetches all invoices from PayPal and upserts them.
// Size-cap exception: sync pipeline — 61L sequential sync stages; splitting creates artificial state-passing overhead.
func syncInvoices(ctx context.Context, pool *pgxpool.Pool, client *PayPalClient, accountID string) (int, error) {
	invoices, err := client.ListInvoices()
	if err != nil {
		return 0, err
	}

	count := 0
	for _, inv := range invoices {
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		createdAt := parseTimePtr(inv.CreateTime)
		updatedAt := parseTimePtr(inv.UpdateTime)

		var amount *float64
		var currency *string
		if inv.Amount != nil {
			a := parseFloat(inv.Amount.Value)
			amount = &a
			currency = &inv.Amount.CurrencyCode
		}

		var dueDate *string
		detail := inv.Detail
		if len(detail) == 0 {
			detail = json.RawMessage("{}")
		}

		// Extract due_date from detail if present.
		var detailMap map[string]interface{}
		if json.Unmarshal(detail, &detailMap) == nil {
			if pt, ok := detailMap["payment_term"].(map[string]interface{}); ok {
				if dd, ok := pt["due_date"].(string); ok {
					dueDate = &dd
				}
			}
		}

		invoicer := inv.Invoicer
		if len(invoicer) == 0 {
			invoicer = json.RawMessage("{}")
		}

		err := UpsertInvoice(dbCtx, pool, &Invoice{
			PayPalID:        inv.ID,
			Status:          inv.Status,
			Detail:          detail,
			Amount:          amount,
			Currency:        currency,
			DueDate:         dueDate,
			Invoicer:        invoicer,
			CreatedAt:       createdAt,
			UpdatedAt:       updatedAt,
			SourceAccountID: accountID,
		})
		cancel()
		if err != nil {
			return count, fmt.Errorf("upsert invoice %s: %w", inv.ID, err)
		}
		count++
	}
	return count, nil
}

// --- Helpers -----------------------------------------------------------------

// buildClients creates PayPalClient instances for each configured account,
// falling back to the primary config if no multi-account config is present.
func buildClients(cfg *Config) map[string]*PayPalClient {
	clients := make(map[string]*PayPalClient)

	if len(cfg.Accounts) > 0 {
		for _, acct := range cfg.Accounts {
			accountCfg := &Config{
				ClientID:     acct.ClientID,
				ClientSecret: acct.ClientSecret,
				Environment:  cfg.Environment,
			}
			label := acct.Label
			if label == "" {
				label = "primary"
			}
			clients[label] = NewPayPalClient(accountCfg)
		}
	} else {
		clients["primary"] = NewPayPalClient(cfg)
	}

	return clients
}

// parseTimePtr parses an RFC3339 time string, returning nil if empty or invalid.
