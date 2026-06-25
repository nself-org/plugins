package internal

import (
	"context"
	"fmt"
	"log"
	"time"
	"github.com/jackc/pgx/v5/pgxpool"
)

func syncAccount(ctx context.Context, pool *pgxpool.Pool, client *PayPalClient, accountID string, result *SyncResult) {
	type syncTask struct {
		name string
		fn   func() (int, error)
	}

	endDate := time.Now().Format(time.RFC3339)
	startDate := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)

	tasks := []syncTask{
		{"products", func() (int, error) { return syncProducts(ctx, pool, client, accountID) }},
		{"subscription_plans", func() (int, error) { return syncSubscriptionPlans(ctx, pool, client, accountID) }},
		{"transactions", func() (int, error) { return syncTransactions(ctx, pool, client, accountID, startDate, endDate) }},
		{"disputes", func() (int, error) { return syncDisputes(ctx, pool, client, accountID) }},
		{"invoices", func() (int, error) { return syncInvoices(ctx, pool, client, accountID) }},
	}

	for _, task := range tasks {
		log.Printf("[nself-paypal] syncing %s for %s", task.name, accountID)
		count, err := task.fn()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s(%s): %s", task.name, accountID, err.Error()))
			result.Success = false
			log.Printf("[nself-paypal] error syncing %s: %v", task.name, err)
		} else {
			result.Synced[task.name] += count
			log.Printf("[nself-paypal] synced %d %s for %s", count, task.name, accountID)
		}
	}
}

// syncProducts fetches all products from PayPal and upserts them.
func syncProducts(ctx context.Context, pool *pgxpool.Pool, client *PayPalClient, accountID string) (int, error) {
	products, err := client.ListProducts()
	if err != nil {
		return 0, err
	}

	count := 0
	for _, p := range products {
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		createdAt := parseTimePtr(p.CreateTime)
		updatedAt := parseTimePtr(p.UpdateTime)
		desc := nilIfEmpty(p.Description)
		cat := nilIfEmpty(p.Category)
		img := nilIfEmpty(p.ImageURL)
		home := nilIfEmpty(p.HomeURL)

		err := UpsertProduct(dbCtx, pool, &Product{
			PayPalID:        p.ID,
			Name:            p.Name,
			Description:     desc,
			Type:            p.Type,
			Category:        cat,
			ImageURL:        img,
			HomeURL:         home,
			CreatedAt:       createdAt,
			UpdatedAt:       updatedAt,
			SourceAccountID: accountID,
		})
		cancel()
		if err != nil {
			return count, fmt.Errorf("upsert product %s: %w", p.ID, err)
		}
		count++
	}
	return count, nil
}

// syncSubscriptionPlans fetches all plans from PayPal and upserts them.
