package store

// Purpose: durable state of the node-global production boundary (P7-NODE-14, Epic D9): the project
// roots the fabric has seen, the cached exclusion entries per project, the refresh state behind the
// doctor warning, and the one write that sets a node's deploy_host fact.
// Inputs:  entries read by deployguard from `nself deploy targets --json` (one root at a time).
// Outputs: ExclusionEntries (the union over every root), ExclusionStates, NodeSetDeployHost.
// Constraints: ExclusionReplace is the only call that changes a root's entries and the caller makes it
//          only after a successful read, so a failed refresh never shrinks the set (ExclusionFail
//          touches the state row only); only ExclusionProjectSeen registers a root; the deploy_host change
//          of a node is one transaction with its audit row.

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

//go:embed migrations/node-exclusion/*.sql
var exclusionMigrations embed.FS

func init() {
	if err := Register("node-exclusion", exclusionMigrations); err != nil {
		panic(err)
	}
}

// ExclusionEntry is one excluded identity: a host name, an address, a CIDR prefix, a host-key
// fingerprint or a machine id, with where it came from (inventory, never-list or marker).
type ExclusionEntry struct {
	Root, Kind, Value, Source string
}

// ExclusionState is the refresh outcome of one project root.
type ExclusionState struct {
	Root                  string
	OK                    bool
	RefreshedAt, LastOKAt int64
	Error                 string
}

// ExclusionProjectSeen records a project root the fabric has seen.
func (s *Store) ExclusionProjectSeen(ctx context.Context, root string) error {
	if root == "" {
		return coded("E607", "invalid project root")
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO excl_project(root,seen_at) VALUES(?,?) ON CONFLICT(root) DO UPDATE SET seen_at=excluded.seen_at`, root, s.now().UnixNano())
		return err
	})
}

// ExclusionProjects lists every known project root.
func (s *Store) ExclusionProjects(ctx context.Context) ([]string, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT root FROM excl_project ORDER BY root`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var r string
		if err = rows.Scan(&r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ExclusionReplace swaps the entries of one root for the result of a successful read.
func (s *Store) ExclusionReplace(ctx context.Context, root string, entries []ExclusionEntry) error {
	if root == "" {
		return coded("E607", "invalid project root")
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		now := s.now().UnixNano()
		var before int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM excl_entry WHERE root=?`, root).Scan(&before); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM excl_entry WHERE root=?`, root); err != nil {
			return err
		}
		for _, e := range entries {
			if _, err := tx.ExecContext(ctx, `INSERT INTO excl_entry(root,kind,value,source) VALUES(?,?,?,?) ON CONFLICT(root,kind,value) DO NOTHING`, root, e.Kind, e.Value, e.Source); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO excl_state(root,refreshed_at,ok,last_ok_at,error) VALUES(?,?,1,?,'') ON CONFLICT(root) DO UPDATE SET refreshed_at=excluded.refreshed_at,ok=1,last_ok_at=excluded.last_ok_at,error=''`, root, now, now); err != nil {
			return err
		}
		var after int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM excl_entry WHERE root=?`, root).Scan(&after); err != nil {
			return err
		}
		if after < before {
			return s.appendAudit(ctx, tx, "deployguard", "exclusion.shrink", root, fmt.Sprintf("entries %d to %d", before, after))
		}
		return nil
	})
}

// ExclusionForget removes a project root with its entries and state, with one audit row.
func (s *Store) ExclusionForget(ctx context.Context, root, actor string) error {
	if root == "" {
		return coded("E607", "invalid project root")
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{`DELETE FROM excl_entry WHERE root=?`, `DELETE FROM excl_state WHERE root=?`, `DELETE FROM excl_project WHERE root=?`} {
			if _, err := tx.ExecContext(ctx, q, root); err != nil {
				return err
			}
		}
		return s.appendAudit(ctx, tx, actor, "exclusion.forget", root, "project forgotten")
	})
}

// ExclusionFail records a failed refresh. The entries of the root are left as they were.
func (s *Store) ExclusionFail(ctx context.Context, root, reason string) error {
	if root == "" {
		return coded("E607", "invalid project root")
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		now := s.now().UnixNano()
		_, err := tx.ExecContext(ctx, `INSERT INTO excl_state(root,refreshed_at,ok,error) VALUES(?,?,0,?) ON CONFLICT(root) DO UPDATE SET refreshed_at=excluded.refreshed_at,ok=0,error=excluded.error`, root, now, reason)
		return err
	})
}

// ExclusionEntries is the union of the cached entries of every root.
func (s *Store) ExclusionEntries(ctx context.Context) ([]ExclusionEntry, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT root,kind,value,source FROM excl_entry ORDER BY root,kind,value`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExclusionEntry
	for rows.Next() {
		var e ExclusionEntry
		if err = rows.Scan(&e.Root, &e.Kind, &e.Value, &e.Source); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ExclusionStates lists the refresh state of every root.
func (s *Store) ExclusionStates(ctx context.Context) ([]ExclusionState, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT root,ok,refreshed_at,last_ok_at,error FROM excl_state ORDER BY root`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExclusionState
	for rows.Next() {
		var st ExclusionState
		var ok int
		if err = rows.Scan(&st.Root, &ok, &st.RefreshedAt, &st.LastOKAt, &st.Error); err != nil {
			return nil, err
		}
		st.OK = ok == 1
		out = append(out, st)
	}
	return out, rows.Err()
}

// NodeSetDeployHost sets the deploy_host fact of a node (the column and the capability document)
// in one transaction with an audit row. An unchanged fact writes nothing.
// version is the node version the caller decided on (0 skips the check): a node changed since then fails
// with E605 and the caller re-evaluates.
func (s *Store) NodeSetDeployHost(ctx context.Context, nodeID string, version int64, d model.CapabilityDeployHost, actor, detail string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		var doc, col string
		var current int64
		err := tx.QueryRowContext(ctx, `SELECT s.doc_json,n.deploy_host_json,n.version FROM node n JOIN capability_snapshot s ON s.node_id=n.id WHERE n.id=?`, nodeID).Scan(&doc, &col, &current)
		if errors.Is(err, sql.ErrNoRows) {
			return coded("E650", "node not found")
		}
		if err != nil {
			return err
		}
		if version != 0 && version != current {
			return coded("E605", "node version changed")
		}
		var c model.Capability
		if err = json.Unmarshal([]byte(doc), &c); err != nil {
			return err
		}
		next, err := json.Marshal(d)
		if err != nil {
			return err
		}
		prev, _ := json.Marshal(c.DeployHost)
		if string(prev) == string(next) && col == string(next) {
			return nil
		}
		c.DeployHost = d
		raw, err := json.Marshal(c)
		if err != nil {
			return err
		}
		now := s.now().UnixNano()
		res, err := tx.ExecContext(ctx, `UPDATE node SET deploy_host_json=?,version=version+1,updated_at=? WHERE id=? AND version=?`, string(next), now, nodeID, current)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return coded("E605", "node version changed")
		}
		if _, err = tx.ExecContext(ctx, `UPDATE capability_snapshot SET digest=?,doc_json=? WHERE node_id=?`, digestNode(raw), string(raw), nodeID); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, actor, "node.deploy_host", nodeID, fmt.Sprintf("%s matched=%t override=%t", detail, d.Matched, d.Override))
	})
}
