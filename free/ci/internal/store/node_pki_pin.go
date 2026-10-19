package store

// Purpose: the compare-and-set write of a node key pin (D-1257).
// Inputs:  node id, the key id the caller vetted, the new pin. Outputs: whether the pin was replaced.
// Constraints: one UPDATE keyed on the vetted key id, so a rotation made since it was read is never undone.

import (
	"context"
	"database/sql"
)

// PKINodeKeyPutIf is PKINodeKeyPut as a compare-and-set: it writes only while the node still has a pin
// whose key id is wantKeyID, and reports whether it did. A rotation made since the pin was read changes
// that key id, so a stale writer cannot undo it.
func (s *Store) PKINodeKeyPutIf(ctx context.Context, nodeID, wantKeyID, keyID, public string) (bool, error) {
	var swapped bool
	err := s.write(ctx, func(tx *sql.Tx) error {
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
