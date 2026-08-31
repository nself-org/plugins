// Package db handles database connectivity and migrations.
package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-payments/internal/provider"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Connect creates a pgx connection pool.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("db: parse config: %w", err)
	}
	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return pool, nil
}

// Migrate runs all SQL migration files in order.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	// Ensure migration tracking table exists.
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS np_payments_migrations (
			filename TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ DEFAULT now()
		)
	`)
	if err != nil {
		return fmt.Errorf("db: create migrations table: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("db: read migrations: %w", err)
	}

	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") && !strings.HasSuffix(e.Name(), ".down.sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, filename := range files {
		var exists bool
		if err := pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM np_payments_migrations WHERE filename=$1)", filename,
		).Scan(&exists); err != nil {
			return fmt.Errorf("db: check migration %s: %w", filename, err)
		}
		if exists {
			continue
		}

		content, err := migrationsFS.ReadFile(filepath.Join("migrations", filename))
		if err != nil {
			return fmt.Errorf("db: read migration %s: %w", filename, err)
		}

		if _, err := pool.Exec(ctx, string(content)); err != nil {
			return fmt.Errorf("db: apply migration %s: %w", filename, err)
		}

		if _, err := pool.Exec(ctx,
			"INSERT INTO np_payments_migrations (filename) VALUES ($1)", filename,
		); err != nil {
			return fmt.Errorf("db: record migration %s: %w", filename, err)
		}
	}
	return nil
}

// UpsertSubscription inserts or updates a subscription row from a canonical provider.Subscription.
func UpsertSubscription(ctx context.Context, pool *pgxpool.Pool, userID, providerName string, sub *provider.Subscription) error {
	const q = `
		INSERT INTO np_subscriptions (
			id, user_id, provider, provider_sub_id, provider_customer_id,
			plan_id, status, current_period_start, current_period_end,
			cancel_at_period_end, trial_end, updated_at
		) VALUES (
			gen_random_uuid(), $1, $2, $3, $4,
			$5, $6, $7, $8,
			$9, $10, now()
		)
		ON CONFLICT (provider_sub_id) DO UPDATE SET
			provider_customer_id  = EXCLUDED.provider_customer_id,
			plan_id               = EXCLUDED.plan_id,
			status                = EXCLUDED.status,
			current_period_start  = EXCLUDED.current_period_start,
			current_period_end    = EXCLUDED.current_period_end,
			cancel_at_period_end  = EXCLUDED.cancel_at_period_end,
			trial_end             = EXCLUDED.trial_end,
			updated_at            = now()
	`
	_, err := pool.Exec(ctx, q,
		userID, providerName, sub.ProviderSubID, sub.ProviderCustomerID,
		sub.PlanID, string(sub.Status), sub.CurrentPeriodStart, sub.CurrentPeriodEnd,
		sub.CancelAtPeriodEnd, sub.TrialEnd,
	)
	return err
}

// GetSubscriptionByUser returns the active subscription for a user.
func GetSubscriptionByUser(ctx context.Context, pool *pgxpool.Pool, userID string) (*DBSubscription, error) {
	const q = `
		SELECT id, user_id, provider, provider_sub_id, provider_customer_id,
			plan_id, status, current_period_start, current_period_end,
			cancel_at_period_end, trial_end, grace_period_end, metadata,
			created_at, updated_at
		FROM np_subscriptions
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	row := pool.QueryRow(ctx, q, userID)
	return scanSubscription(row)
}

// GetPastDueSubscriptions returns subscriptions in past_due state for dunning.
func GetPastDueSubscriptions(ctx context.Context, pool *pgxpool.Pool) ([]DBSubscription, error) {
	const q = `
		SELECT id, user_id, provider, provider_sub_id, provider_customer_id,
			plan_id, status, current_period_start, current_period_end,
			cancel_at_period_end, trial_end, grace_period_end, metadata,
			created_at, updated_at
		FROM np_subscriptions
		WHERE status = 'past_due'
		ORDER BY updated_at ASC
	`
	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []DBSubscription
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		subs = append(subs, *sub)
	}
	return subs, rows.Err()
}

// SetGracePeriodEnd updates the grace_period_end for a subscription.
func SetGracePeriodEnd(ctx context.Context, pool *pgxpool.Pool, subID uuid.UUID, graceEnd time.Time) error {
	_, err := pool.Exec(ctx,
		"UPDATE np_subscriptions SET grace_period_end = $1, updated_at = now() WHERE id = $2",
		graceEnd, subID,
	)
	return err
}

// GetExpiredGracePeriods returns subscriptions past grace period end.
func GetExpiredGracePeriods(ctx context.Context, pool *pgxpool.Pool) ([]DBSubscription, error) {
	const q = `
		SELECT id, user_id, provider, provider_sub_id, provider_customer_id,
			plan_id, status, current_period_start, current_period_end,
			cancel_at_period_end, trial_end, grace_period_end, metadata,
			created_at, updated_at
		FROM np_subscriptions
		WHERE status = 'past_due' AND grace_period_end IS NOT NULL AND grace_period_end < now()
	`
	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var subs []DBSubscription
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		subs = append(subs, *sub)
	}
	return subs, rows.Err()
}

// RecordUsage inserts a usage record.
func RecordUsage(ctx context.Context, pool *pgxpool.Pool, subID uuid.UUID, metricID string, quantity int) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO np_usage_records (id, subscription_id, metric_id, quantity) VALUES (gen_random_uuid(), $1, $2, $3)",
		subID, metricID, quantity,
	)
	return err
}

// GetUsageSummary returns total units for a subscription+metric in the current period.
func GetUsageSummary(ctx context.Context, pool *pgxpool.Pool, subID uuid.UUID, metricID string) (int, error) {
	var total int
	err := pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(quantity),0) FROM np_usage_records
		 WHERE subscription_id = $1 AND metric_id = $2
		   AND recorded_at >= date_trunc('month', now())`,
		subID, metricID,
	).Scan(&total)
	return total, err
}

// InsertPaymentEvent inserts a raw webhook event for audit.
func InsertPaymentEvent(ctx context.Context, pool *pgxpool.Pool, providerName, eventType string, payload []byte) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO np_payment_events (id, provider, event_type, payload) VALUES (gen_random_uuid(), $1, $2, $3)",
		providerName, eventType, payload,
	)
	return err
}

// MarkPaymentEventProcessed marks a payment event as processed.
func MarkPaymentEventProcessed(ctx context.Context, pool *pgxpool.Pool, eventID uuid.UUID, errMsg string) error {
	var e *string
	if errMsg != "" {
		e = &errMsg
	}
	_, err := pool.Exec(ctx,
		"UPDATE np_payment_events SET processed = true, error = $1 WHERE id = $2",
		e, eventID,
	)
	return err
}

// QueueDepth returns unprocessed payment events count.
func QueueDepth(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var n int64
	err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM np_payment_events WHERE processed = false").Scan(&n)
	return n, err
}

// IdempotencyEntry is a cached CreateCheckout result keyed by request UUID.
type IdempotencyEntry struct {
	RequestUUID string
	SessionID   string
	SessionURL  string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

// GetCheckoutIdempotency looks up a prior CreateCheckout result by request UUID.
// Returns nil, nil when no live (non-expired) entry exists.
func GetCheckoutIdempotency(ctx context.Context, pool *pgxpool.Pool, requestUUID string) (*IdempotencyEntry, error) {
	const q = `
		SELECT request_uuid, session_id, session_url, created_at, expires_at
		FROM   np_stripe_idempotency
		WHERE  request_uuid = $1
		  AND  expires_at   > now()
	`
	var e IdempotencyEntry
	err := pool.QueryRow(ctx, q, requestUUID).Scan(
		&e.RequestUUID, &e.SessionID, &e.SessionURL, &e.CreatedAt, &e.ExpiresAt,
	)
	if err != nil {
		// pgx returns pgx.ErrNoRows when nothing is found; treat as cache miss.
		return nil, nil //nolint:nilerr
	}
	return &e, nil
}

// SetCheckoutIdempotency persists a (request_uuid, session_id, session_url) mapping
// with a 24-hour TTL. Conflicts on request_uuid are ignored — the first writer wins,
// which is the correct idempotency semantic.
func SetCheckoutIdempotency(ctx context.Context, pool *pgxpool.Pool, requestUUID, sessionID, sessionURL string) error {
	const q = `
		INSERT INTO np_stripe_idempotency (request_uuid, session_id, session_url)
		VALUES ($1, $2, $3)
		ON CONFLICT (request_uuid) DO NOTHING
	`
	_, err := pool.Exec(ctx, q, requestUUID, sessionID, sessionURL)
	return err
}

// PurgeExpiredIdempotencyKeys deletes rows past their 24-hour TTL.
// Call periodically (e.g. from a cron or at startup after Migrate).
func PurgeExpiredIdempotencyKeys(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tag, err := pool.Exec(ctx, "DELETE FROM np_stripe_idempotency WHERE expires_at <= now()")
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// DBSubscription is the DB representation of np_subscriptions.
type DBSubscription struct {
	ID                 uuid.UUID
	UserID             uuid.UUID
	Provider           string
	ProviderSubID      string
	ProviderCustomerID string
	PlanID             string
	Status             string
	CurrentPeriodStart time.Time
	CurrentPeriodEnd   time.Time
	CancelAtPeriodEnd  bool
	TrialEnd           *time.Time
	GracePeriodEnd     *time.Time
	Metadata           []byte
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type scannable interface {
	Scan(dest ...interface{}) error
}

func scanSubscription(row scannable) (*DBSubscription, error) {
	var s DBSubscription
	err := row.Scan(
		&s.ID, &s.UserID, &s.Provider, &s.ProviderSubID, &s.ProviderCustomerID,
		&s.PlanID, &s.Status, &s.CurrentPeriodStart, &s.CurrentPeriodEnd,
		&s.CancelAtPeriodEnd, &s.TrialEnd, &s.GracePeriodEnd, &s.Metadata,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}
