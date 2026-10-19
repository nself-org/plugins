package store

// Purpose: per-node contention settings (P7-NODE-13, Epic D10): reserved CPU and memory, interactive
// protection, the on-battery policy and persistence. One row per node in migration set `node-settings`.
// Inputs:  a NodeSettings value from `nself ci nodes set` (an admin command), or a node id to read.
// Outputs: the stored row; the node's capability snapshot carries the same values as assigned facts
//          (resources.reserved, availability.persistence|on_battery|interactive_policy), so the
//          scheduler (P7-SCHED-01) reads them from the one snapshot.
// Constraints: only a coordinator-side admin writes these; UpdateCapability keeps them across agent
//          reports. The row, the snapshot and the audit row commit in one transaction. ReservedCPU is
//          in cores, the unit of resources.cpu.

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

//go:embed migrations/node-settings/*.sql
var nodeSettingsMigrations embed.FS

func init() {
	if err := Register("node-settings", nodeSettingsMigrations); err != nil {
		panic(err)
	}
}

// The allowed values of the policy fields.
const (
	InteractiveProtect = "protect"
	InteractiveIgnore  = "ignore"
	OnBatteryPause     = "pause"
	OnBatteryDrain     = "drain"
	OnBatteryAllow     = "allow"
	PersistencePersist = "persistent"
	PersistenceInterm  = "intermittent"
)

// NodeSettings is the operator's contention policy for one node.
type NodeSettings struct {
	NodeID        string  `json:"node"`
	ReservedCPU   float64 `json:"reserved_cpu"`
	ReservedMemMB int64   `json:"reserved_mem_mb"`
	Interactive   string  `json:"interactive"`
	OnBattery     string  `json:"on_battery"`
	Persistence   string  `json:"persistence"`
	Version       int64   `json:"version"`
	UpdatedAt     int64   `json:"updated_at"`
}

// Validate rejects a value outside the closed sets or a negative or non-finite reservation.
func (n NodeSettings) Validate() error {
	switch {
	case n.NodeID == "":
		return coded("E651", "node id required")
	case math.IsNaN(n.ReservedCPU) || math.IsInf(n.ReservedCPU, 0) || n.ReservedCPU < 0:
		return coded("E651", "reserved cpu must be 0 or more cores")
	case n.ReservedMemMB < 0:
		return coded("E651", "reserved memory must be 0 or more MB")
	case !slices.Contains([]string{InteractiveProtect, InteractiveIgnore}, n.Interactive):
		return coded("E651", "interactive must be protect or ignore")
	case !slices.Contains([]string{OnBatteryPause, OnBatteryDrain, OnBatteryAllow}, n.OnBattery):
		return coded("E651", "on_battery must be pause, drain or allow")
	case !slices.Contains([]string{PersistencePersist, PersistenceInterm}, n.Persistence):
		return coded("E651", "persistence must be persistent or intermittent")
	}
	return nil
}

// NodeSettingsGet returns the stored settings; ok is false when the operator never set any.
func (s *Store) NodeSettingsGet(ctx context.Context, nodeID string) (NodeSettings, bool, error) {
	var n NodeSettings
	err := s.readers.QueryRowContext(ctx, `SELECT node_id,reserved_cpu,reserved_mem_mb,interactive,on_battery,persistence,version,updated_at FROM node_settings WHERE node_id=?`, nodeID).
		Scan(&n.NodeID, &n.ReservedCPU, &n.ReservedMemMB, &n.Interactive, &n.OnBattery, &n.Persistence, &n.Version, &n.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return NodeSettings{}, false, nil
	}
	return n, err == nil, err
}

// NodePutSettings stores in for its node and mirrors it into the capability snapshot as assigned facts.
// The node must exist (E650). One transaction covers the row, the snapshot and the audit row.
func (s *Store) NodePutSettings(ctx context.Context, in NodeSettings, actor string) (NodeSettings, error) {
	if err := in.Validate(); err != nil {
		return NodeSettings{}, err
	}
	if actor == "" || actor == "agent" || actor == "probe" {
		return NodeSettings{}, coded("E651", "admin authority required")
	}
	var out NodeSettings
	err := s.write(ctx, func(tx *sql.Tx) error {
		var doc string
		var version int64
		if err := tx.QueryRowContext(ctx, `SELECT n.version,c.doc_json FROM node n JOIN capability_snapshot c ON c.node_id=n.id WHERE n.id=?`, in.NodeID).Scan(&version, &doc); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return coded("E650", "node not found")
			}
			return err
		}
		now := s.now().UTC()
		var c model.Capability
		if err := json.Unmarshal([]byte(doc), &c); err != nil {
			return err
		}
		applySettings(&c, in, now)
		raw, err := json.Marshal(c)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO node_settings(node_id,reserved_cpu,reserved_mem_mb,interactive,on_battery,persistence,version,updated_at) VALUES(?,?,?,?,?,?,1,?)
			ON CONFLICT(node_id) DO UPDATE SET reserved_cpu=excluded.reserved_cpu,reserved_mem_mb=excluded.reserved_mem_mb,interactive=excluded.interactive,on_battery=excluded.on_battery,persistence=excluded.persistence,version=node_settings.version+1,updated_at=excluded.updated_at`,
			in.NodeID, in.ReservedCPU, in.ReservedMemMB, in.Interactive, in.OnBattery, in.Persistence, now.UnixNano()); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE node SET version=version+1,updated_at=? WHERE id=? AND version=?`, now.UnixNano(), in.NodeID, version)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return coded("E605", "node version changed")
		}
		if _, err = tx.ExecContext(ctx, `UPDATE capability_snapshot SET digest=?,doc_json=? WHERE node_id=?`, digestNode(raw), string(raw), in.NodeID); err != nil {
			return err
		}
		if err = s.appendAudit(ctx, tx, actor, "node.settings", in.NodeID, "reserved/interactive/on_battery/persistence set"); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, `SELECT node_id,reserved_cpu,reserved_mem_mb,interactive,on_battery,persistence,version,updated_at FROM node_settings WHERE node_id=?`, in.NodeID).
			Scan(&out.NodeID, &out.ReservedCPU, &out.ReservedMemMB, &out.Interactive, &out.OnBattery, &out.Persistence, &out.Version, &out.UpdatedAt)
	})
	return out, err
}

// assignedFact is a known fact the coordinator assigned.
func assignedFact[T any](v T, now time.Time) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: "assigned", ObservedAt: now, Confidence: "known"}
}

// applySettings writes the settings into the capability document as assigned, known facts.
func applySettings(c *model.Capability, in NodeSettings, now time.Time) {
	c.Resources.Reserved.CPU = assignedFact(in.ReservedCPU, now)
	c.Resources.Reserved.MemMB = assignedFact(in.ReservedMemMB, now)
	c.Availability.Persistence = assignedFact(in.Persistence == PersistencePersist, now)
	c.Availability.OnBattery = assignedFact(in.OnBattery, now)
	c.Availability.InteractivePolicy = assignedFact(in.Interactive, now)
}
