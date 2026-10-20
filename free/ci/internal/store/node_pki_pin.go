package store

// Purpose: the compare-and-set write of a node key pin (D-1257).
// Inputs:  node id, the key id the caller vetted, the new pin. Outputs: whether the pin was replaced.
// Constraints: one UPDATE keyed on the vetted key id, so a rotation made since it was read is never undone.

import (
	"context"
	"database/sql"
	"errors"
)

// PKINodeKeyPutIf is PKINodeKeyPut as a compare-and-set: it writes only while the node still has a pin
// whose key id is wantKeyID, and reports whether it did. A rotation made since the pin was read changes
// that key id, so a stale writer cannot undo it.
func (s *Store) PKINodeKeyPutIf(ctx context.Context, nodeID, wantKeyID, keyID, public string) (bool, error) {
	var swapped bool
	err := s.write(ctx, func(tx *sql.Tx) error {
		if err := refuseSharedNodeKey(ctx, tx, nodeID, keyID, public); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE pki_node_key SET key_id=?,public=?,pinned_at=? WHERE node_id=? AND key_id=?`, keyID, public, s.now().UnixNano(), nodeID, wantKeyID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		swapped = n == 1
		return err
	})
	return swapped, err
}

// PKINodeKeyPut pins a node's Ed25519 key, replacing any earlier pin.
func (s *Store) PKINodeKeyPut(ctx context.Context, nodeID, keyID, public string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		if err := refuseSharedNodeKey(ctx, tx, nodeID, keyID, public); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO pki_node_key(node_id,key_id,public,pinned_at) VALUES(?,?,?,?)`, nodeID, keyID, public, s.now().UnixNano())
		return err
	})
}

// ErrNodeKeyBound refuses a key used by another node without exposing either key.
var ErrNodeKeyBound = errors.New("node key is already bound to another node")

type nodeKeyQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func refuseSharedNodeKey(ctx context.Context, q nodeKeyQuery, nodeID, keyID, public string) error {
	var count int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM pki_node_key WHERE node_id<>? AND public<>'' AND (key_id=? OR public=?)`, nodeID, keyID, public).Scan(&count)
	if err != nil {
		return err
	}
	if count != 0 {
		return ErrNodeKeyBound
	}
	return nil
}

// PKINodeKeyExclusive also rejects duplicate bindings left by older coordinators.
func (s *Store) PKINodeKeyExclusive(ctx context.Context, nodeID, keyID, public string) error {
	return refuseSharedNodeKey(ctx, s.readers, nodeID, keyID, public)
}
