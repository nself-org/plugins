package internal

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SyncAll performs a full sync of all PayPal data: products, subscription plans,
// transactions (in 31-day windows), subscriptions, disputes, invoices, and payouts.
// For multi-account setups, it iterates over all configured accounts.
func SyncAll(ctx context.Context, pool *pgxpool.Pool, cfg *Config) *SyncResult {
	started := time.Now()
	result := &SyncResult{
		Success: true,
		Synced:  make(map[string]int),
	}

	clients := buildClients(cfg)

	for label, client := range clients {
		accountID := label
		if accountID == "" {
			accountID = "primary"
		}

		log.Printf("[nself-paypal] syncing account: %s", accountID)

		syncAccount(ctx, pool, client, accountID, result)
	}

	result.Duration = time.Since(started).String()
	return result
}

// Reconcile performs a recent-data sync (last N days) to catch gaps from missed webhooks.
func Reconcile(ctx context.Context, pool *pgxpool.Pool, cfg *Config, lookbackDays int) *SyncResult {
	started := time.Now()
	result := &SyncResult{
		Success: true,
		Synced:  make(map[string]int),
	}

	if lookbackDays <= 0 {
		lookbackDays = 7
	}

	clients := buildClients(cfg)
	since := time.Now().Add(-time.Duration(lookbackDays) * 24 * time.Hour)

	for label, client := range clients {
		accountID := label
		if accountID == "" {
			accountID = "primary"
		}

		log.Printf("[nself-paypal] reconciling account: %s (since %s)", accountID, since.Format(time.RFC3339))

		count, err := syncTransactions(ctx, pool, client, accountID, since.Format(time.RFC3339), time.Now().Format(time.RFC3339))
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("transactions(%s): %s", accountID, err.Error()))
			result.Success = false
		} else {
			result.Synced["transactions"] += count
		}

		dCount, err := syncDisputes(ctx, pool, client, accountID)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("disputes(%s): %s", accountID, err.Error()))
			result.Success = false
		} else {
			result.Synced["disputes"] += dCount
		}
	}

	result.Duration = time.Since(started).String()
	return result
}

// syncAccount runs the full sync pipeline for a single PayPal account.
