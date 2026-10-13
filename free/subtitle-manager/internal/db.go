package internal

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool with subtitle-manager table operations.
type DB struct {
	pool *pgxpool.Pool
}

// NewDB creates a new DB wrapper.
func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool}
}

// ---------------------------------------------------------------------------
// Subtitles CRUD
// ---------------------------------------------------------------------------

// SearchSubtitles finds locally stored subtitles for a given media and language.
