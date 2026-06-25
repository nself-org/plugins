package internal

import (
	"context"
	"fmt"
	"time"
	"github.com/jackc/pgx/v5/pgxpool"
)

func syncSubscriptionPlans(ctx context.Context, pool *pgxpool.Pool, client *PayPalClient, accountID string) (int, error) {
	plans, err := client.ListSubscriptionPlans()
	if err != nil {
		return 0, err
	}

	count := 0
	for _, p := range plans {
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		createdAt := parseTimePtr(p.CreateTime)
		updatedAt := parseTimePtr(p.UpdateTime)
		desc := nilIfEmpty(p.Description)

		err := UpsertSubscriptionPlan(dbCtx, pool, &SubscriptionPlan{
			PayPalID:           p.ID,
			ProductID:          p.ProductID,
			Name:               p.Name,
			Description:        desc,
			Status:             p.Status,
			BillingCycles:      p.BillingCycles,
			PaymentPreferences: p.PaymentPreferences,
			CreatedAt:          createdAt,
			UpdatedAt:          updatedAt,
			SourceAccountID:    accountID,
		})
		cancel()
		if err != nil {
			return count, fmt.Errorf("upsert plan %s: %w", p.ID, err)
		}
		count++
	}
	return count, nil
}

// syncTransactions searches PayPal transactions in 31-day windows and upserts them.
// Size-cap exception: sync pipeline — 68L sequential sync stages; splitting creates artificial state-passing overhead.
func syncTransactions(ctx context.Context, pool *pgxpool.Pool, client *PayPalClient, accountID, startDate, endDate string) (int, error) {
	details, err := client.SearchTransactions(startDate, endDate)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, d := range details {
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		info := d.TransactionInfo

		var fee *float64
		if info.FeeAmount != nil {
			f := parseFloat(info.FeeAmount.Value)
			fee = &f
		}

		amount := float64(0)
		currency := "USD"
		if info.TransactionAmount != nil {
			amount = parseFloat(info.TransactionAmount.Value)
			currency = info.TransactionAmount.CurrencyCode
		}

		net := amount
		if fee != nil {
			v := amount - *fee
			net = v
		}
		netPtr := &net

		var payerEmail, payerName *string
		if d.PayerInfo != nil {
			payerEmail = d.PayerInfo.EmailAddress
			if d.PayerInfo.PayerName != nil {
				name := joinNames(d.PayerInfo.PayerName.GivenName, d.PayerInfo.PayerName.Surname)
				if name != "" {
					payerName = &name
				}
			}
		}

		createdAt := parseTimePtr(info.TransactionInitiationDate)
		updatedAt := parseTimePtr(info.TransactionUpdatedDate)

		err := UpsertTransaction(dbCtx, pool, &Transaction{
			PayPalID:        info.TransactionID,
			Type:            info.TransactionEventCode,
			Status:          info.TransactionStatus,
			Amount:          amount,
			Currency:        currency,
			Fee:             fee,
			Net:             netPtr,
			PayerEmail:      payerEmail,
			PayerName:       payerName,
			Description:     info.TransactionSubject,
			CreatedAt:       createdAt,
			UpdatedAt:       updatedAt,
			SourceAccountID: accountID,
		})
		cancel()
		if err != nil {
			return count, fmt.Errorf("upsert transaction %s: %w", info.TransactionID, err)
		}
		count++
	}
	return count, nil
}

// syncDisputes fetches all disputes from PayPal and upserts them.
