package store

import "github.com/jackc/pgx/v5/pgxpool"

// Pool returns the underlying pool (boot migrations run on it).
func (db *DB) Pool() *pgxpool.Pool { return db.pool }
