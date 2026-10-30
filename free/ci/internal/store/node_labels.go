package store

// Purpose: operator-assigned node labels (P7-NODE-30) and their per-key audit trail, in migration set
// `node-labels`. The placement label source (internal/nodes/labels) reads them into the world.
// Inputs:  a node id, the full desired label set and the acting operator.
// Outputs: the stored labels sorted by key; one audit row per key that was set, changed or removed.
// Constraints: the store checks shape only (non-empty node, actor and keys, no duplicate key); key and
//          value syntax, the reserved prefix and the count limit belong to internal/nodes/labels (E658).
//          A replace that changes nothing writes no audit rows. Labels and audit commit in one transaction.

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
)

//go:embed migrations/node-labels/*.sql
var nodeLabelsMigrations embed.FS

func init() {
	if err := Register("node-labels", nodeLabelsMigrations); err != nil {
		panic(err)
	}
}

// Label audit actions.
const (
	LabelSet    = "set"
	LabelChange = "change"
	LabelRemove = "remove"
)

// NodeLabel is one assigned label; an empty Value is a bare key label.
type NodeLabel struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// NodeLabelChange is one audit row of a label edit.
type NodeLabelChange struct {
	Seq      int64  `json:"seq"`
	NodeID   string `json:"node"`
	Actor    string `json:"actor"`
	Action   string `json:"action"`
	Key      string `json:"key"`
	OldValue string `json:"old_value"`
	Value    string `json:"value"`
	At       int64  `json:"at"`
}

// NodeLabelsReplace makes set the node's whole label set and records each difference as an audit row.
// Shape errors (empty node, actor or key, a duplicate key) are E607.
func (s *Store) NodeLabelsReplace(ctx context.Context, nodeID string, set []NodeLabel, actor string) error {
	if nodeID == "" || actor == "" {
		return coded("E607", "node id and actor required")
	}
	want := map[string]string{}
	for _, l := range set {
		if l.Key == "" {
			return coded("E607", "label key required")
		}
		if _, dup := want[l.Key]; dup {
			return coded("E607", "duplicate label key "+l.Key)
		}
		want[l.Key] = l.Value
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		have, err := queryLabels(ctx, tx, nodeID)
		if err != nil {
			return err
		}
		old := map[string]string{}
		for _, l := range have {
			old[l.Key] = l.Value
		}
		keys := make([]string, 0, len(old)+len(want))
		for k := range old {
			keys = append(keys, k)
		}
		for k := range want {
			if _, ok := old[k]; !ok {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		now := s.now().UnixNano()
		counts := map[string]int{}
		for _, k := range keys {
			was, had := old[k]
			is, keep := want[k]
			var action string
			switch {
			case had && !keep:
				action = LabelRemove
				_, err = tx.ExecContext(ctx, `DELETE FROM node_label WHERE node_id=? AND key=?`, nodeID, k)
			case !had:
				action = LabelSet
				_, err = tx.ExecContext(ctx, `INSERT INTO node_label(node_id,key,value,updated_at) VALUES(?,?,?,?)`, nodeID, k, is, now)
			case was != is:
				action = LabelChange
				_, err = tx.ExecContext(ctx, `UPDATE node_label SET value=?,updated_at=? WHERE node_id=? AND key=?`, is, now, nodeID, k)
			default:
				continue
			}
			if err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO node_label_audit(node_id,actor,action,key,old_value,value,at) VALUES(?,?,?,?,?,?,?)`,
				nodeID, actor, action, k, was, is, now); err != nil {
				return err
			}
			counts[action]++
		}
		if len(counts) == 0 {
			return nil
		}
		detail := fmt.Sprintf("set=%d change=%d remove=%d", counts[LabelSet], counts[LabelChange], counts[LabelRemove])
		return s.appendAudit(ctx, tx, actor, "node.labels", nodeID, detail)
	})
}

// NodeLabels returns the node's labels sorted by key; an unknown node has none.
func (s *Store) NodeLabels(ctx context.Context, nodeID string) ([]NodeLabel, error) {
	return queryLabels(ctx, s.readers, nodeID)
}

// NodeLabelsAll returns every labeled node's labels, keyed by node id.
func (s *Store) NodeLabelsAll(ctx context.Context) (map[string][]NodeLabel, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT node_id,key,value FROM node_label ORDER BY node_id,key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]NodeLabel{}
	for rows.Next() {
		var id string
		var l NodeLabel
		if err = rows.Scan(&id, &l.Key, &l.Value); err != nil {
			return nil, err
		}
		out[id] = append(out[id], l)
	}
	return out, rows.Err()
}

// PipelineProjects returns every project id a pipeline was recorded for, sorted: the projects this store has seen.
func (s *Store) PipelineProjects(ctx context.Context) ([]string, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT DISTINCT project FROM pipeline WHERE project<>'' ORDER BY project`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var p string
		if err = rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// NodeLabelAudit returns the node's label audit rows, oldest first.
func (s *Store) NodeLabelAudit(ctx context.Context, nodeID string) ([]NodeLabelChange, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT seq,node_id,actor,action,key,old_value,value,at FROM node_label_audit WHERE node_id=? ORDER BY seq`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NodeLabelChange
	for rows.Next() {
		var c NodeLabelChange
		if err = rows.Scan(&c.Seq, &c.NodeID, &c.Actor, &c.Action, &c.Key, &c.OldValue, &c.Value, &c.At); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type labelQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func queryLabels(ctx context.Context, q labelQuerier, nodeID string) ([]NodeLabel, error) {
	rows, err := q.QueryContext(ctx, `SELECT key,value FROM node_label WHERE node_id=? ORDER BY key`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NodeLabel{}
	for rows.Next() {
		var l NodeLabel
		if err = rows.Scan(&l.Key, &l.Value); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
