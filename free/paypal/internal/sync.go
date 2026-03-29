package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
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
func parseTimePtr(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		// Try RFC3339Nano as well.
		t, err = time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return nil
		}
	}
	return &t
}

// parseFloat converts a string to float64, returning 0 on failure.
func parseFloat(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// nilIfEmpty returns nil if the string is empty, otherwise a pointer to it.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// joinNames concatenates non-nil name parts with a space.
func joinNames(parts ...*string) string {
	var result []string
	for _, p := range parts {
		if p != nil && *p != "" {
			result = append(result, *p)
		}
	}
	if len(result) == 0 {
		return ""
	}
	out := result[0]
	for i := 1; i < len(result); i++ {
		out += " " + result[i]
	}
	return out
}
