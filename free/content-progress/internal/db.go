package internal

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool with content-progress table operations.
type DB struct {
	pool                 *pgxpool.Pool
	sourceAccountID      string
	completeThreshold    int
	historySampleSeconds int

	mu                sync.Mutex
	lastHistoryInsert map[string]time.Time
}

// NewDB creates a new DB wrapper with default source account.
func NewDB(pool *pgxpool.Pool, cfg Config) *DB {
	return &DB{
		pool:                 pool,
		sourceAccountID:      "primary",
		completeThreshold:    cfg.CompleteThreshold,
		historySampleSeconds: cfg.HistorySampleSeconds,
		lastHistoryInsert:    make(map[string]time.Time),
	}
}

// ForSourceAccount returns a new DB scoped to the given source account ID.
// It shares the same pool, config, and history sampling state.
func (d *DB) ForSourceAccount(sourceAccountID string) *DB {
	return &DB{
		pool:                 d.pool,
		sourceAccountID:      normalizeSourceAccountID(sourceAccountID),
		completeThreshold:    d.completeThreshold,
		historySampleSeconds: d.historySampleSeconds,
		lastHistoryInsert:    d.lastHistoryInsert,
		mu:                   sync.Mutex{},
	}
}

var nonAlphanumRegex = regexp.MustCompile(`[^a-z0-9_-]+`)
var leadTrailDash = regexp.MustCompile(`^-+|-+$`)

func normalizeSourceAccountID(value string) string {
	normalized := strings.ToLower(value)
	normalized = nonAlphanumRegex.ReplaceAllString(normalized, "-")
	normalized = leadTrailDash.ReplaceAllString(normalized, "")
	if normalized == "" {
		return "primary"
	}
	return normalized
}

// Ping verifies the database connection is alive.
func (d *DB) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := d.pool.Exec(ctx, "SELECT 1")
	return err
}
