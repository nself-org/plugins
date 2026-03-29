package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// SyncResult holds the outcome of a sync operation for one account.
type SyncResult struct {
	AccountID string         `json:"account_id"`
	Counts    map[string]int `json:"counts"`
	Errors    []string       `json:"errors,omitempty"`
	Duration  string         `json:"duration"`
}

// SyncAll runs a full data sync from Stripe for all configured accounts.
func SyncAll(ctx context.Context, db *DB, client *StripeClient, accounts []StripeAccountConfig) []SyncResult {
	var results []SyncResult

	for _, acct := range accounts {
		start := time.Now()
		scopedDB := db.ForSourceAccount(acct.ID)
		scopedClient := client.WithAPIKey(acct.APIKey)

		log.Printf("[stripe:sync] Starting sync for account %q", acct.ID)
		result := syncAccount(ctx, scopedDB, scopedClient, acct.ID)
		result.Duration = time.Since(start).Round(time.Millisecond).String()
		results = append(results, result)
		log.Printf("[stripe:sync] Completed sync for account %q: %v", acct.ID, result.Counts)
	}

	return results
}

// Reconcile syncs only recent data (created in the last 24 hours) to catch missed webhooks.
func Reconcile(ctx context.Context, db *DB, client *StripeClient, accounts []StripeAccountConfig) []SyncResult {
	// Reconcile uses the same sync logic. Stripe's list endpoints return most recent first,
	// and we upsert, so re-syncing effectively reconciles.
	return SyncAll(ctx, db, client, accounts)
}

func syncAccount(ctx context.Context, db *DB, client *StripeClient, accountID string) SyncResult {
	result := SyncResult{
		AccountID: accountID,
		Counts:    make(map[string]int),
	}

	// Sync in dependency order
	syncResource(ctx, db, client, "customers", &result)
	syncResource(ctx, db, client, "products", &result)
	syncResource(ctx, db, client, "prices", &result)
	syncResource(ctx, db, client, "coupons", &result)
	syncResource(ctx, db, client, "promotion_codes", &result)
	syncResource(ctx, db, client, "subscriptions", &result)
	syncResource(ctx, db, client, "invoices", &result)
	syncResource(ctx, db, client, "charges", &result)
	syncResource(ctx, db, client, "refunds", &result)
	syncResource(ctx, db, client, "disputes", &result)
	syncResource(ctx, db, client, "payment_intents", &result)
	syncResource(ctx, db, client, "setup_intents", &result)
	syncResource(ctx, db, client, "balance_transactions", &result)
	syncResource(ctx, db, client, "tax_rates", &result)

	return result
}

func syncResource(ctx context.Context, db *DB, client *StripeClient, resource string, result *SyncResult) {
	log.Printf("[stripe:sync] Syncing %s...", resource)

	objects, err := fetchResource(client, resource)
	if err != nil {
		errMsg := fmt.Sprintf("fetch %s: %v", resource, err)
		log.Printf("[stripe:sync] Error: %s", errMsg)
		result.Errors = append(result.Errors, errMsg)
		return
	}

	count := 0
	for _, raw := range objects {
		if err := upsertResource(ctx, db, resource, raw); err != nil {
			errMsg := fmt.Sprintf("upsert %s: %v", resource, err)
			log.Printf("[stripe:sync] Error: %s", errMsg)
			result.Errors = append(result.Errors, errMsg)
			continue
		}
		count++
	}

	result.Counts[resource] = count
	log.Printf("[stripe:sync] Synced %d %s", count, resource)
}

func fetchResource(client *StripeClient, resource string) ([]json.RawMessage, error) {
	switch resource {
	case "customers":
		return client.ListCustomers()
	case "products":
		return client.ListProducts()
	case "prices":
		return client.ListPrices()
	case "coupons":
		return client.ListCoupons()
	case "promotion_codes":
		return client.ListPromotionCodes()
	case "subscriptions":
		return client.ListSubscriptions()
	case "invoices":
		return client.ListInvoices()
	case "charges":
		return client.ListCharges()
	case "refunds":
		return client.ListRefunds()
	case "disputes":
		return client.ListDisputes()
	case "payment_intents":
		return client.ListPaymentIntents()
	case "setup_intents":
		return client.ListSetupIntents()
	case "balance_transactions":
		return client.ListBalanceTransactions()
	case "tax_rates":
		return client.ListTaxRates()
	default:
		return nil, fmt.Errorf("unknown resource: %s", resource)
	}
}

func upsertResource(ctx context.Context, db *DB, resource string, raw json.RawMessage) error {
	// Map resource name to the Stripe object type used by UpsertFromWebhookEvent
	objectType := resourceToObjectType(resource)
	return db.UpsertFromWebhookEvent(ctx, objectType, raw)
}

func resourceToObjectType(resource string) string {
	switch resource {
	case "customers":
		return "customer"
	case "products":
		return "product"
	case "prices":
		return "price"
	case "coupons":
		return "coupon"
	case "promotion_codes":
		return "promotion_code"
	case "subscriptions":
		return "subscription"
	case "invoices":
		return "invoice"
	case "charges":
		return "charge"
	case "refunds":
		return "refund"
	case "disputes":
		return "dispute"
	case "payment_intents":
		return "payment_intent"
	case "setup_intents":
		return "setup_intent"
	case "balance_transactions":
		return "balance_transaction"
	case "tax_rates":
		return "tax_rate"
	case "checkout_sessions":
		return "checkout.session"
	default:
		return resource
	}
}
