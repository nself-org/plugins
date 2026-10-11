package internal

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool}
}

// RecordJobRun inserts a history row for one dispatch attempt.
