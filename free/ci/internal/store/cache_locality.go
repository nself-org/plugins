package store

// Purpose: persist advisory cache-copy locality for the scheduler.
// Inputs: node identity, realm, entry key, byte count and last verification time.
// Outputs: isolated, per-realm locality rows; no cache write capability.
// Constraints: locality never authorizes a restore or bypasses digest verification.

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"time"
)

//go:embed migrations/cache-locality/*.sql
var cacheLocalityMigrations embed.FS

func init() {
	if err := Register("cache-locality", cacheLocalityMigrations); err != nil {
		panic(err)
	}
}

// CacheLocalityRow describes a node's verified copy of an entry in one realm.
type CacheLocalityRow struct {
	Node, Realm, Key string
	Bytes            int64
	LastSeen         time.Time
}

// CacheLocalityPut records or refreshes one copy. Zero LastSeen uses the store clock.
func (s *Store) CacheLocalityPut(ctx context.Context, row CacheLocalityRow) error {
	if row.Node == "" || row.Realm == "" || row.Key == "" || row.Bytes < 0 {
		return errors.New("store: invalid cache locality metadata")
	}
	if row.LastSeen.IsZero() {
		row.LastSeen = s.now()
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO cache_locality(node,realm,cache_key,bytes,last_seen)
VALUES (?,?,?,?,?) ON CONFLICT(node,realm,cache_key) DO UPDATE SET bytes=excluded.bytes,last_seen=excluded.last_seen`,
			row.Node, row.Realm, row.Key, row.Bytes, row.LastSeen.UnixNano())
		return err
	})
}

// CacheLocality returns copies matching the nonempty filters in stable order.
func (s *Store) CacheLocality(ctx context.Context, node, realm, key string) ([]CacheLocalityRow, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT node,realm,cache_key,bytes,last_seen FROM cache_locality
WHERE (?='' OR node=?) AND (?='' OR realm=?) AND (?='' OR cache_key=?) ORDER BY node,realm,cache_key`, node, node, realm, realm, key, key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []CacheLocalityRow
	for rows.Next() {
		var row CacheLocalityRow
		var seen int64
		if err := rows.Scan(&row.Node, &row.Realm, &row.Key, &row.Bytes, &seen); err != nil {
			return nil, err
		}
		row.LastSeen = time.Unix(0, seen)
		out = append(out, row)
	}
	return out, rows.Err()
}

// CacheLocalityDrop forgets one copy after eviction or a failed verification.
func (s *Store) CacheLocalityDrop(ctx context.Context, node, realm, key string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM cache_locality WHERE node=? AND realm=? AND cache_key=?", node, realm, key)
		return err
	})
}
