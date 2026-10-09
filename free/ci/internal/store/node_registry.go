package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
)

//go:embed migrations/node-registry/*.sql
var nodeMigrations embed.FS

func init() {
	if err := Register("node-registry", nodeMigrations); err != nil {
		panic(err)
	}
}

// NodeRecord is the store aggregate; JSON columns are opaque to SQL.
type NodeRecord struct {
	ID, Name, State, Reason                  string
	Version                                  int64
	Lifecycle, Trust, DeployHost, Capability json.RawMessage
	Digest                                   string
	ObservedAt, CreatedAt, UpdatedAt         int64
}

func digestNode(doc []byte) string { sum := sha256.Sum256(doc); return hex.EncodeToString(sum[:]) }

// NodePut registers a node and its first snapshot in one audited transaction.
func (s *Store) NodePut(ctx context.Context, n NodeRecord, actor string) error {
	if n.ID == "" || n.Name == "" || len(n.Capability) == 0 {
		return coded("E607", "invalid node")
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		now := s.now().UnixNano()
		_, err := tx.ExecContext(ctx, `INSERT INTO node(id,name,version,lifecycle,trust_json,deploy_host_json,state,reason,created_at,updated_at) VALUES(?,?,1,?,?,?,?,?,?,?)`, n.ID, n.Name, string(n.Lifecycle), string(n.Trust), string(n.DeployHost), n.State, n.Reason, now, now)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO capability_snapshot(node_id,digest,doc_json,observed_at) VALUES(?,?,?,?)`, n.ID, digestNode(n.Capability), string(n.Capability), n.ObservedAt)
		if err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, actor, "node.register", n.ID, "registered")
	})
}

func scanNode(row interface{ Scan(...any) error }) (NodeRecord, error) {
	var n NodeRecord
	var lifecycle, trust, host, doc string
	err := row.Scan(&n.ID, &n.Name, &n.Version, &lifecycle, &trust, &host, &n.State, &n.Reason, &n.CreatedAt, &n.UpdatedAt, &n.Digest, &doc, &n.ObservedAt)
	n.Lifecycle, n.Trust, n.DeployHost, n.Capability = []byte(lifecycle), []byte(trust), []byte(host), []byte(doc)
	return n, err
}

const nodeSelect = `SELECT n.id,n.name,n.version,n.lifecycle,n.trust_json,n.deploy_host_json,n.state,n.reason,n.created_at,n.updated_at,c.digest,c.doc_json,c.observed_at FROM node n JOIN capability_snapshot c ON c.node_id=n.id`

func (s *Store) NodeGet(ctx context.Context, id string) (NodeRecord, error) {
	n, err := scanNode(s.readers.QueryRowContext(ctx, nodeSelect+` WHERE n.id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return n, coded("E650", "node not found")
	}
	return n, err
}
func (s *Store) NodeList(ctx context.Context) ([]NodeRecord, error) {
	rows, err := s.readers.QueryContext(ctx, nodeSelect+` ORDER BY n.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NodeRecord{}
	for rows.Next() {
		n, e := scanNode(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// nodeCAS commits an aggregate replacement and one audit row, or E605 on a stale version.
func (s *Store) nodeCAS(ctx context.Context, n NodeRecord, actor, action, detail string, updateSnapshot bool) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE node SET name=?,version=version+1,lifecycle=?,trust_json=?,deploy_host_json=?,state=?,reason=?,updated_at=? WHERE id=? AND version=?`, n.Name, string(n.Lifecycle), string(n.Trust), string(n.DeployHost), n.State, n.Reason, s.now().UnixNano(), n.ID, n.Version)
		if err != nil {
			return err
		}
		changed, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return coded("E605", "node version changed")
		}
		if updateSnapshot {
			_, err = tx.ExecContext(ctx, `UPDATE capability_snapshot SET digest=?,doc_json=?,observed_at=? WHERE node_id=?`, digestNode(n.Capability), string(n.Capability), n.ObservedAt, n.ID)
			if err != nil {
				return err
			}
		}
		return s.appendAudit(ctx, tx, actor, action, n.ID, detail)
	})
}
func (s *Store) NodeUpdateCapability(ctx context.Context, n NodeRecord, actor string) error {
	return s.nodeCAS(ctx, n, actor, "node.capability", "capability updated", true)
}
func (s *Store) NodeSetTrust(ctx context.Context, n NodeRecord, actor string) error {
	return s.nodeCAS(ctx, n, actor, "node.trust", "trust assigned", true)
}
func (s *Store) NodeSetState(ctx context.Context, n NodeRecord, actor, detail string) error {
	if detail != "age_recipient_changed" {
		detail = "state changed"
	}
	return s.nodeCAS(ctx, n, actor, "node.state", detail, true)
}
func (s *Store) NodeSetAuthorization(ctx context.Context, n NodeRecord, actor string) error {
	return s.nodeCAS(ctx, n, actor, "node.authorization", "project authorization changed", true)
}
