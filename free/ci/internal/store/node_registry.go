package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

//go:embed migrations/node-registry/*.sql
var nodeMigrations embed.FS

func init() {
	if err := Register("node-registry", nodeMigrations); err != nil {
		panic(err)
	}
}

type NodeRecord struct {
	ID, Name, State, Reason                  string
	Version                                  int64
	Lifecycle, Trust, DeployHost, Capability json.RawMessage
	Digest                                   string
	ObservedAt, CreatedAt, UpdatedAt         int64
}

// AdminAuthority is an explicit coordinator decision for trust and recovery.
type AdminAuthority struct{ actor string }

func NewAdminAuthority(actor string) (AdminAuthority, error) {
	if actor == "" || actor == "agent" || actor == "probe" {
		return AdminAuthority{}, coded("E651", "admin authority required")
	}
	return AdminAuthority{actor: actor}, nil
}
func (a AdminAuthority) valid() bool {
	return a.actor != "" && a.actor != "agent" && a.actor != "probe"
}
func (a AdminAuthority) Actor() string { return a.actor }
func unknownNodeFact[T any](now time.Time) model.Fact[T] {
	return model.Fact[T]{Source: "assigned", ObservedAt: now, Confidence: "unknown"}
}
func initialNodeTrust(now time.Time) model.CapabilityTrust {
	empty := []model.TrustClass{}
	accepts := model.Fact[[]model.TrustClass]{Value: &empty, Source: "assigned", ObservedAt: now, Confidence: "known"}
	return model.CapabilityTrust{Accepts: accepts, Isolation: unknownNodeFact[model.Isolation](now), Network: unknownNodeFact[[]model.NetworkScope](now), SecretClasses: unknownNodeFact[[]model.SecretClass](now), PrivacyZone: unknownNodeFact[model.PrivacyZone](now)}
}
func digestNode(doc []byte) string { sum := sha256.Sum256(doc); return hex.EncodeToString(sum[:]) }

// NodePut registers a node and its first snapshot in one audited transaction.
func (s *Store) NodePut(ctx context.Context, n NodeRecord, actor string) error {
	if n.ID == "" || n.Name == "" || len(n.Capability) == 0 {
		return coded("E607", "invalid node")
	}
	var c model.Capability
	if json.Unmarshal(n.Capability, &c) == nil && c.Schema == "ci.runner-capability/v1" {
		c.Identity.Ownership = model.Fact[string]{Source: "assigned", ObservedAt: s.now().UTC(), Confidence: "unknown"}
		c.Trust = initialNodeTrust(s.now().UTC())
		if err := nodeDocument(&n, c); err != nil {
			return err
		}
		var err error
		n.DeployHost, err = json.Marshal(c.DeployHost)
		if err != nil {
			return err
		}
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

func (s *Store) nodeCAS(ctx context.Context, n NodeRecord, actor, action, detail string, isolationLowered bool) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		query := `UPDATE node SET name=?,version=version+1,updated_at=? WHERE id=? AND version=?`
		args := []any{n.Name, s.now().UnixNano(), n.ID, n.Version}
		if action == "node.capability" && isolationLowered {
			query = `UPDATE node SET name=?,version=version+1,trust_json=?,updated_at=? WHERE id=? AND version=?`
			args = []any{n.Name, string(n.Trust), s.now().UnixNano(), n.ID, n.Version}
		} else if action != "node.capability" {
			query = `UPDATE node SET name=?,version=version+1,lifecycle=?,trust_json=?,deploy_host_json=?,state=?,reason=?,updated_at=? WHERE id=? AND version=?`
			args = []any{n.Name, string(n.Lifecycle), string(n.Trust), string(n.DeployHost), n.State, n.Reason, s.now().UnixNano(), n.ID, n.Version}
		}
		res, err := tx.ExecContext(ctx, query, args...)
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
		_, err = tx.ExecContext(ctx, `UPDATE capability_snapshot SET digest=?,doc_json=?,observed_at=? WHERE node_id=?`, digestNode(n.Capability), string(n.Capability), n.ObservedAt, n.ID)
		if err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, actor, action, n.ID, detail)
	})
}
func (s *Store) nodeEdit(ctx context.Context, proposed NodeRecord) (NodeRecord, model.Capability, error) {
	old, err := s.NodeGet(ctx, proposed.ID)
	if err != nil {
		return old, model.Capability{}, err
	}
	if old.Version != proposed.Version {
		return old, model.Capability{}, coded("E605", "node version changed")
	}
	var c model.Capability
	err = json.Unmarshal(old.Capability, &c)
	return old, c, err
}
func nodeDocument(n *NodeRecord, c model.Capability) error {
	var err error
	n.Capability, err = json.Marshal(c)
	if err != nil {
		return err
	}
	n.Lifecycle, err = json.Marshal(c.Lifecycle)
	if err != nil {
		return err
	}
	n.Trust, err = json.Marshal(c.Trust)
	return err
}
func (s *Store) NodeUpdateCapability(ctx context.Context, n NodeRecord, actor string) error {
	old, previous, err := s.nodeEdit(ctx, n)
	if err != nil {
		return err
	}
	var incoming model.Capability
	if err := json.Unmarshal(n.Capability, &incoming); err != nil {
		return err
	}
	if incoming.Identity.ID != old.ID {
		return coded("E651", "node identity changed")
	}
	if previous.Identity.AgeRecipient.Value != nil {
		incoming.Identity.AgeRecipient = previous.Identity.AgeRecipient
	}
	incoming.Identity.Ownership = previous.Identity.Ownership
	reportedIsolation := incoming.Trust.Isolation
	incoming.Trust = previous.Trust
	isolationLowered := false
	if reportedIsolation.Value != nil && previous.Trust.Isolation.Value != nil {
		values := model.EnumValues("Isolation")
		reportedRank := slices.Index(values, string(*reportedIsolation.Value))
		if reportedRank >= 0 && reportedRank < slices.Index(values, string(*previous.Trust.Isolation.Value)) {
			incoming.Trust.Isolation = reportedIsolation
			incoming.Trust.Isolation.Source = "assigned"
			isolationLowered = true
		}
	}
	incoming.Lifecycle = previous.Lifecycle
	incoming.DeployHost = previous.DeployHost
	incoming.Resources.Reserved = previous.Resources.Reserved
	incoming.Availability.Persistence = previous.Availability.Persistence
	incoming.Availability.OnBattery = previous.Availability.OnBattery
	incoming.Availability.InteractivePolicy = previous.Availability.InteractivePolicy
	incoming.Availability.State = previous.Availability.State
	incoming.SecretsEligible = incoming.Separation.UIDSeparation.Value != nil && *incoming.Separation.UIDSeparation.Value
	if err := model.ValidateCapability(incoming); err != nil {
		return err
	}
	if err := nodeDocument(&old, incoming); err != nil {
		return err
	}
	old.ObservedAt, old.Name = n.ObservedAt, incoming.Identity.Name
	return s.nodeCAS(ctx, old, actor, "node.capability", "capability updated", isolationLowered)
}
func (s *Store) NodeSetTrust(ctx context.Context, n NodeRecord, authority AdminAuthority) error {
	if !authority.valid() {
		return coded("E651", "admin authority required")
	}
	old, previous, err := s.nodeEdit(ctx, n)
	if err != nil {
		return err
	}
	var incoming model.Capability
	if err := json.Unmarshal(n.Capability, &incoming); err != nil {
		return err
	}
	previous.Trust = incoming.Trust
	if err := model.ValidateCapability(previous); err != nil {
		return err
	}
	if err := nodeDocument(&old, previous); err != nil {
		return err
	}
	return s.nodeCAS(ctx, old, authority.actor, "node.trust", "trust assigned", false)
}
func (s *Store) NodeSetState(ctx context.Context, n NodeRecord, actor, detail string) error {
	old, c, err := s.nodeEdit(ctx, n)
	if err == nil {
		if c.Lifecycle.RevokedAt != nil && n.State != "revoked" {
			return coded("E651", "revoked node requires admin recovery")
		}
		c.Availability.State = model.Fact[string]{Value: &n.State, Source: "assigned", ObservedAt: time.Now().UTC(), Confidence: "known"}
		if n.State == "revoked" && c.Lifecycle.RevokedAt == nil {
			now := time.Now().UTC()
			c.Lifecycle.RevokedAt = &now
		}
		if err := nodeDocument(&old, c); err != nil {
			return err
		}
	} else if old.Version != n.Version || old.ID != n.ID {
		return err
	}
	old.State, old.Reason = n.State, n.Reason
	if detail != "age_recipient_changed" {
		detail = "state changed"
	}
	return s.nodeCAS(ctx, old, actor, "node.state", detail, false)
}

func (s *Store) NodeRecover(ctx context.Context, n NodeRecord, authority AdminAuthority) error {
	if !authority.valid() {
		return coded("E651", "admin authority required")
	}
	if n.State != "offline" && n.State != "maintenance" {
		return coded("E651", "invalid recovery state")
	}
	old, c, err := s.nodeEdit(ctx, n)
	if err != nil {
		return err
	}
	if c.Lifecycle.RevokedAt == nil {
		return coded("E651", "node is not revoked")
	}
	c.Lifecycle.RevokedAt = nil
	c.Availability.State = model.Fact[string]{Value: &n.State, Source: "assigned", ObservedAt: time.Now().UTC(), Confidence: "known"}
	if err := nodeDocument(&old, c); err != nil {
		return err
	}
	old.State, old.Reason = n.State, "admin recovery"
	return s.nodeCAS(ctx, old, authority.actor, "node.recover", "admin recovery", false)
}
func (s *Store) NodeSetAuthorization(ctx context.Context, n NodeRecord, actor string) error {
	old, previous, err := s.nodeEdit(ctx, n)
	if err != nil {
		return err
	}
	var incoming model.Capability
	if err := json.Unmarshal(n.Capability, &incoming); err != nil {
		return err
	}
	previous.Lifecycle.AuthorizedProjects = incoming.Lifecycle.AuthorizedProjects
	if err := nodeDocument(&old, previous); err != nil {
		return err
	}
	return s.nodeCAS(ctx, old, actor, "node.authorization", "project authorization changed", false)
}
