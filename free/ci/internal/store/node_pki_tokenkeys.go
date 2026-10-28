package store

// Purpose: the sealed job-token keys of the coordinator PKI (P7-NODE-07): store, first-key creation, read.
// Inputs:  a key id, the public key and the sealed seed. Outputs: the stored keys, newest first.
// Constraints: the seed is stored only sealed under the KEK; the first key is one conditional insert.

import (
	"context"
	"database/sql"
)

// PKITokenKeyPut stores a sealed token key.
func (s *Store) PKITokenKeyPut(ctx context.Context, kid, public string, sealed []byte) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO pki_token_key(kid,public,key_sealed,created_at) VALUES(?,?,?,?)`, kid, public, sealed, s.now().UnixNano())
		return err
	})
}

// PKITokenKeyPutFirst stores a sealed token key only when the table is empty, in one write transaction,
// and reports whether it did. Two processes bootstrapping at once end with one key, not two.
func (s *Store) PKITokenKeyPutFirst(ctx context.Context, kid, public string, sealed []byte) (bool, error) {
	var put bool
	err := s.write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO pki_token_key(kid,public,key_sealed,created_at) SELECT ?,?,?,? WHERE NOT EXISTS (SELECT 1 FROM pki_token_key)`, kid, public, sealed, s.now().UnixNano())
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		put = n == 1
		return err
	})
	return put, err
}

// PKITokenKey is a stored job-token key (Sealed: AES-GCM sealed Ed25519 seed).
type PKITokenKey struct {
	KID, Public string
	Sealed      []byte
	Retired     bool
}

func (s *Store) PKITokenKeys(ctx context.Context) ([]PKITokenKey, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT kid,public,key_sealed,retired_at>0 FROM pki_token_key ORDER BY created_at DESC,kid`)
	return pkiCollect(rows, err, func(r *sql.Rows) (k PKITokenKey, e error) { return k, r.Scan(&k.KID, &k.Public, &k.Sealed, &k.Retired) })
}
