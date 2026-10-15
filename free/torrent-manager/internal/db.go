package internal

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool with torrent-manager table operations.
type DB struct {
	pool             *pgxpool.Pool
	torrentListLimit int
}

// NewDB creates a new DB wrapper.
func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool, torrentListLimit: 200}
}

// NewDBWithLimit creates a new DB wrapper with a custom torrent list limit.
func NewDBWithLimit(pool *pgxpool.Pool, limit int) *DB {
	if limit <= 0 {
		limit = 200
	}
	return &DB{pool: pool, torrentListLimit: limit}
}
