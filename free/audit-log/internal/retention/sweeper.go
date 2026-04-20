// Package retention implements the audit-log partition sweeper.
//
// S44-T10: Daily cron drops monthly partitions older than the configured
// retention window. Per-tier override is supported:
//   - Enterprise: 7 years (2555 days)
//   - Business:   1 year  (365 days)
//   - Default:    90 days (AUDIT_LOG_RETENTION_DAYS env var)
//
// GDPR Art 17 anonymization: PII fields (actor_user_id, ip_address, user_agent)
// are cleared on rows that exceed the retention window but whose partition is
// still current (partition-level drop handles older ones). Transaction history
// is preserved keyed by an anonymized author_id.
//
// Security: sweeper runs with minimal Postgres privileges. It NEVER truncates;
// it only drops partitions or NULLs PII columns. Failures are logged but do not
// block the main service.
package retention

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// defaultRetentionDays is the base retention window for non-Enterprise,
	// non-Business installations.
	defaultRetentionDays = 90

	// enterpriseRetentionDays is the retention window for Enterprise tier
	// customers (7 years).
	enterpriseRetentionDays = 365 * 7

	// businessRetentionDays is the retention window for Business tier
	// customers (1 year).
	businessRetentionDays = 365

	// partitionTablePrefix is the naming convention for monthly partitions.
	// Tables are named np_auditlog_events_YYYY_MM.
	partitionTablePrefix = "np_auditlog_events_"
)

// Tier represents the nSelf subscription tier that governs retention.
type Tier string

const (
	TierDefault    Tier = "default"
	TierBusiness   Tier = "business"
	TierEnterprise Tier = "enterprise"
)

// Sweeper holds the state for the retention sweeper.
type Sweeper struct {
	pool          *pgxpool.Pool
	retentionDays int
}

// New creates a Sweeper using the given pool and tier.
//
// Retention days are determined by (in priority order):
//  1. Tier override (enterprise=2555, business=365)
//  2. AUDIT_LOG_RETENTION_DAYS env var (default=90)
func New(pool *pgxpool.Pool, tier Tier) *Sweeper {
	days := retentionDaysForTier(tier)
	return &Sweeper{pool: pool, retentionDays: days}
}

// retentionDaysForTier returns the effective retention window.
func retentionDaysForTier(tier Tier) int {
	switch tier {
	case TierEnterprise:
		return enterpriseRetentionDays
	case TierBusiness:
		return businessRetentionDays
	default:
		// Check env override for non-tiered installations.
		if v := os.Getenv("AUDIT_LOG_RETENTION_DAYS"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				return n
			}
		}
		return defaultRetentionDays
	}
}

// RetentionDays returns the effective retention window (exposed for testing).
func (s *Sweeper) RetentionDays() int {
	return s.retentionDays
}

// Sweep executes one sweep cycle:
//  1. Drops monthly partitions older than the retention window (O(1), no
//     table scan).
//  2. Anonymizes PII in the current/recent partition for rows whose created_at
//     is past the retention cutoff but whose partition cannot yet be dropped
//     (e.g., a partition that spans both retained and expired rows).
//
// Returns the number of partitions dropped and any fatal error.
func (s *Sweeper) Sweep(ctx context.Context) (droppedPartitions int, err error) {
	cutoff := time.Now().AddDate(0, 0, -s.retentionDays)

	dropped, dropErr := s.dropExpiredPartitions(ctx, cutoff)
	if dropErr != nil {
		slog.Error("audit-log sweeper: partition drop failed",
			"err", dropErr,
			"cutoff", cutoff.Format("2006-01"),
		)
		return dropped, dropErr
	}

	anonErr := s.anonymizePII(ctx, cutoff)
	if anonErr != nil {
		slog.Error("audit-log sweeper: PII anonymization failed",
			"err", anonErr,
			"cutoff", cutoff.Format(time.RFC3339),
		)
		return dropped, anonErr
	}

	slog.Info("audit-log sweeper: sweep complete",
		"partitions_dropped", dropped,
		"cutoff", cutoff.Format("2006-01"),
		"retention_days", s.retentionDays,
	)
	return dropped, nil
}

// dropExpiredPartitions drops np_auditlog_events_YYYY_MM tables whose month
// is entirely before the cutoff. Partition drop is O(1) in Postgres.
func (s *Sweeper) dropExpiredPartitions(ctx context.Context, cutoff time.Time) (int, error) {
	// List partitions of the audit-log parent table.
	rows, err := s.pool.Query(ctx, `
		SELECT child.relname
		FROM   pg_inherits
		JOIN   pg_class parent  ON pg_inherits.inhparent = parent.oid
		JOIN   pg_class child   ON pg_inherits.inhrelid  = child.oid
		WHERE  parent.relname = 'np_auditlog_events'
		  AND  child.relname  LIKE 'np_auditlog_events_%'
		ORDER  BY child.relname
	`)
	if err != nil {
		return 0, fmt.Errorf("listing partitions: %w", err)
	}
	defer rows.Close()

	var partitions []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return 0, fmt.Errorf("scanning partition name: %w", err)
		}
		partitions = append(partitions, name)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterating partitions: %w", err)
	}

	dropped := 0
	for _, p := range partitions {
		partitionTime, parseErr := partitionMonth(p)
		if parseErr != nil {
			slog.Warn("audit-log sweeper: skipping unrecognized partition", "name", p)
			continue
		}
		// A partition is fully expired when the end of its month is before cutoff.
		partitionEnd := partitionTime.AddDate(0, 1, 0) // first day of next month
		if partitionEnd.Before(cutoff) {
			if _, execErr := s.pool.Exec(ctx,
				fmt.Sprintf("DROP TABLE IF EXISTS %s", p)); execErr != nil {
				slog.Error("audit-log sweeper: drop failed", "partition", p, "err", execErr)
				continue
			}
			slog.Info("audit-log sweeper: dropped partition", "partition", p)
			dropped++
		}
	}
	return dropped, nil
}

// anonymizePII NULLs PII fields on rows in the parent table (or retained
// partitions) whose created_at is past the retention cutoff.
// Fields cleared: actor_user_id → anonymized hash, ip_address → NULL,
// user_agent → NULL.
// This satisfies GDPR Art 17 "right to erasure" while preserving audit
// trail integrity (event type, resource, severity, timestamps remain).
func (s *Sweeper) anonymizePII(ctx context.Context, cutoff time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE np_auditlog_events
		SET    actor_user_id = 'anon-' || encode(sha256(actor_user_id::bytea), 'hex'),
		       ip_address    = NULL,
		       user_agent    = NULL
		WHERE  created_at    < $1
		  AND  actor_user_id NOT LIKE 'anon-%'
	`, cutoff)
	if err != nil {
		return fmt.Errorf("anonymizing PII: %w", err)
	}
	return nil
}

// partitionMonth parses a partition name of the form np_auditlog_events_YYYY_MM
// and returns the first day of that month.
func partitionMonth(name string) (time.Time, error) {
	// Expected format: np_auditlog_events_2024_03
	suffix := name[len(partitionTablePrefix):]
	t, err := time.Parse("2006_01", suffix)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing partition month %q: %w", suffix, err)
	}
	return t, nil
}
