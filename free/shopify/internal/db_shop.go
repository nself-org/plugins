package internal

import (
	"context"
)

// -------------------------------------------------------------------------
// Shop
// -------------------------------------------------------------------------

// UpsertShop inserts or updates a shop record.
func (db *DB) UpsertShop(ctx context.Context, s *shopifyShop) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO np_shopify_shops (shopify_id, name, email, domain, myshopify_domain, country, currency, timezone, plan_name, plan_display_name, money_format, source_account_id, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
		ON CONFLICT (shopify_id, source_account_id) DO UPDATE SET
			name              = EXCLUDED.name,
			email             = EXCLUDED.email,
			domain            = EXCLUDED.domain,
			myshopify_domain  = EXCLUDED.myshopify_domain,
			country           = EXCLUDED.country,
			currency          = EXCLUDED.currency,
			timezone          = EXCLUDED.timezone,
			plan_name         = EXCLUDED.plan_name,
			plan_display_name = EXCLUDED.plan_display_name,
			money_format      = EXCLUDED.money_format,
			updated_at        = NOW(),
			synced_at         = NOW()
	`, s.ID, s.Name, s.Email, s.Domain, s.MyshopifyDomain, s.Country, s.Currency, s.Timezone, s.PlanName, s.PlanDisplayName, s.MoneyFormat, db.SourceAccountID)
	return err
}

// GetShop returns the first shop for this account.
func (db *DB) GetShop(ctx context.Context) (*Shop, error) {
	var s Shop
	err := db.Pool.QueryRow(ctx, `
		SELECT id, shopify_id, name, email, domain, myshopify_domain, country, currency, timezone, plan_name, plan_display_name, money_format, created_at, updated_at, source_account_id, synced_at
		FROM np_shopify_shops WHERE source_account_id = $1 ORDER BY created_at LIMIT 1
	`, db.SourceAccountID).Scan(&s.ID, &s.ShopifyID, &s.Name, &s.Email, &s.Domain, &s.MyshopifyDomain, &s.Country, &s.Currency, &s.Timezone, &s.PlanName, &s.PlanDisplayName, &s.MoneyFormat, &s.CreatedAt, &s.UpdatedAt, &s.SourceAccountID, &s.SyncedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

