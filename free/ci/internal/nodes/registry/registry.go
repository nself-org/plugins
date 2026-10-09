package registry

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Registry owns node decisions; SQL remains in the store package.
type Registry struct{ Store *store.Store }

func New(s *store.Store) *Registry { return &Registry{Store: s} }

type Node struct {
	Record     store.NodeRecord `json:"record"`
	Capability model.Capability `json:"capability"`
}

func decode(n store.NodeRecord) (Node, error) {
	var c model.Capability
	err := json.Unmarshal(n.Capability, &c)
	return Node{n, c}, err
}
func encode(c model.Capability) json.RawMessage { b, _ := json.Marshal(c); return b }
func assigned[T any](v *T) model.Fact[T] {
	return model.Fact[T]{Value: v, Source: "assigned", ObservedAt: time.Now().UTC(), Confidence: "unknown"}
}
func safeTrust() model.CapabilityTrust {
	accepts := []model.TrustClass{}
	f := assigned(&accepts)
	f.Confidence = "known"
	return model.CapabilityTrust{Accepts: f, Isolation: assigned[model.Isolation](nil), Network: assigned[[]model.NetworkScope](nil), SecretClasses: assigned[[]model.SecretClass](nil), PrivacyZone: assigned[model.PrivacyZone](nil)}
}
func eligible(c model.Capability) bool {
	return c.Separation.UIDSeparation.Value != nil && *c.Separation.UIDSeparation.Value
}

// Register records discovery without authorizing a project or accepting node trust.
func (r *Registry) Register(ctx context.Context, c model.Capability, actor string) (Node, error) {
	if c.Identity.ID == "" {
		return Node{}, invalid("/identity/id")
	}
	c.Trust = safeTrust()
	c.Identity.AgeRecipient = model.Fact[string]{Source: "self", ObservedAt: time.Now().UTC(), Confidence: "unknown"}
	c.Lifecycle = model.CapabilityLifecycle{Discovered: true, AuthorizedProjects: []string{}, Eligible: false}
	c.SecretsEligible = eligible(c)
	if err := model.ValidateCapability(c); err != nil {
		return Node{}, invalid(err.Error())
	}
	lifecycle, _ := json.Marshal(c.Lifecycle)
	trust, _ := json.Marshal(c.Trust)
	host, _ := json.Marshal(c.DeployHost)
	n := store.NodeRecord{ID: c.Identity.ID, Name: c.Identity.Name, State: "offline", Lifecycle: lifecycle, Trust: trust, DeployHost: host, Capability: encode(c), ObservedAt: time.Now().UnixNano()}
	if err := r.Store.NodePut(ctx, n, actor); err != nil {
		return Node{}, err
	}
	return r.Get(ctx, n.ID)
}
func (r *Registry) Get(ctx context.Context, id string) (Node, error) {
	n, err := r.Store.NodeGet(ctx, id)
	if err != nil {
		return Node{}, err
	}
	return decode(n)
}
func (r *Registry) List(ctx context.Context) ([]Node, error) {
	rows, err := r.Store.NodeList(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Node, 0, len(rows))
	for _, row := range rows {
		n, e := decode(row)
		if e != nil {
			return nil, e
		}
		out = append(out, n)
	}
	return out, nil
}

// mutate retries a stale snapshot, preserving every committed version and audit row.
func (r *Registry) mutate(ctx context.Context, id, actor, kind string, fn func(*Node) error) (Node, error) {
	for i := 0; i < 256; i++ {
		n, err := r.Get(ctx, id)
		if err != nil {
			return Node{}, err
		}
		if err = fn(&n); err != nil {
			return Node{}, err
		}
		n.Record.Capability = encode(n.Capability)
		n.Record.Lifecycle, _ = json.Marshal(n.Capability.Lifecycle)
		n.Record.Trust, _ = json.Marshal(n.Capability.Trust)
		n.Record.DeployHost, _ = json.Marshal(n.Capability.DeployHost)
		switch kind {
		case "capability":
			err = r.Store.NodeUpdateCapability(ctx, n.Record, actor)
		case "trust":
			err = r.Store.NodeSetTrust(ctx, n.Record, actor)
		case "authorization":
			err = r.Store.NodeSetAuthorization(ctx, n.Record, actor)
		default:
			err = r.Store.NodeSetState(ctx, n.Record, actor, n.Record.Reason)
		}
		var conflict *store.Error
		if errors.As(err, &conflict) && conflict.Code == "E605" {
			continue
		}
		if err != nil {
			return Node{}, err
		}
		return r.Get(ctx, id)
	}
	return Node{}, &store.Error{Code: "E605", Message: "node contention"}
}

// UpdateCapability discards all node-supplied trust, identity pin and lifecycle fields.
// Isolation can only move down the strength ladder.
func (r *Registry) UpdateCapability(ctx context.Context, id string, in model.Capability, actor string) (Node, []string, error) {
	var drift []string
	payload := in
	n, err := r.mutate(ctx, id, actor, "capability", func(n *Node) error {
		in = payload
		drift = nil
		old := n.Capability
		reportedIsolation := in.Trust.Isolation
		if in.Identity.ID != id {
			return invalid("/identity/id")
		}
		if !reflect.DeepEqual(old.Identity.Labels, in.Identity.Labels) {
			drift = append(drift, "/identity/labels")
		}
		if !reflect.DeepEqual(old.Tools, in.Tools) {
			drift = append(drift, "/tools")
		}
		in.Trust = old.Trust
		in.Lifecycle = old.Lifecycle
		in.DeployHost = old.DeployHost
		in.Identity.AgeRecipient = old.Identity.AgeRecipient
		in.Identity.Ownership = old.Identity.Ownership
		in.Resources.Reserved = old.Resources.Reserved
		in.Availability.Persistence = old.Availability.Persistence
		in.Availability.OnBattery = old.Availability.OnBattery
		in.Availability.InteractivePolicy = old.Availability.InteractivePolicy
		in.SecretsEligible = eligible(in)
		if reportedIsolation.Value != nil && old.Trust.Isolation.Value != nil && isolationRank(*reportedIsolation.Value) < isolationRank(*old.Trust.Isolation.Value) {
			in.Trust.Isolation = reportedIsolation
			in.Trust.Isolation.Source = "assigned"
		}
		if err := model.ValidateCapability(in); err != nil {
			return invalid(err.Error())
		}
		n.Capability = in
		n.Record.ObservedAt = time.Now().UnixNano()
		return nil
	})
	return n, drift, err
}
func isolationRank(i model.Isolation) int {
	for n, v := range model.EnumValues("Isolation") {
		if v == string(i) {
			return n
		}
	}
	return -1
}

// SetTrust is the only authority for coordinator-assigned trust fields.
func (r *Registry) SetTrust(ctx context.Context, id string, t model.CapabilityTrust, actor string) (Node, error) {
	if actor == "" {
		return Node{}, invalid("admin actor required")
	}
	return r.mutate(ctx, id, actor, "trust", func(n *Node) error {
		t.Accepts.Source = "assigned"
		t.Isolation.Source = "assigned"
		t.Network.Source = "assigned"
		t.SecretClasses.Source = "assigned"
		t.PrivacyZone.Source = "assigned"
		n.Capability.Trust = t
		return model.ValidateCapability(n.Capability)
	})
}
func (r *Registry) Authorize(ctx context.Context, id, project, actor string) (Node, error) {
	if project == "" {
		return Node{}, invalid("project required")
	}
	return r.mutate(ctx, id, actor, "authorization", func(n *Node) error {
		for _, p := range n.Capability.Lifecycle.AuthorizedProjects {
			if p == project {
				return invalid("project already authorized")
			}
		}
		n.Capability.Lifecycle.AuthorizedProjects = append(n.Capability.Lifecycle.AuthorizedProjects, project)
		return nil
	})
}
func (r *Registry) Deauthorize(ctx context.Context, id, project, actor string) (Node, error) {
	return r.mutate(ctx, id, actor, "authorization", func(n *Node) error {
		out := []string{}
		for _, p := range n.Capability.Lifecycle.AuthorizedProjects {
			if p != project {
				out = append(out, p)
			}
		}
		if len(out) == len(n.Capability.Lifecycle.AuthorizedProjects) {
			return invalid("project not authorized")
		}
		n.Capability.Lifecycle.AuthorizedProjects = out
		return nil
	})
}
func (r *Registry) SetState(ctx context.Context, id, state, reason, actor string) (Node, error) {
	if state != "online" && state != "offline" && state != "draining" && state != "maintenance" && state != "revoked" {
		return Node{}, invalid("state")
	}
	return r.mutate(ctx, id, actor, "state", func(n *Node) error {
		n.Record.State = state
		n.Record.Reason = reason
		n.Capability.Availability.State = model.Fact[string]{Value: &state, Source: "assigned", ObservedAt: time.Now().UTC(), Confidence: "known"}
		if state == "revoked" {
			now := time.Now().UTC()
			n.Capability.Lifecycle.RevokedAt = &now
		}
		return nil
	})
}

// AgeRecipient returns the recipient pinned by the first hello, if any.
func (r *Registry) AgeRecipient(ctx context.Context, id string) (*string, error) {
	n, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return n.Capability.Identity.AgeRecipient.Value, nil
}

// CheckAgeRecipient pins once, then flags a changed recipient without replacing it.
func (r *Registry) CheckAgeRecipient(ctx context.Context, id, presented, actor string) error {
	if presented == "" {
		return invalid("age recipient required")
	}
	n, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	if n.Capability.Identity.AgeRecipient.Value != nil && *n.Capability.Identity.AgeRecipient.Value != presented {
		_, err = r.SetState(ctx, id, "maintenance", "age_recipient_changed", actor)
		if err != nil {
			return err
		}
		return &Error{Code: "E660", Reason: "age_recipient_changed"}
	}
	if n.Capability.Identity.AgeRecipient.Value != nil {
		return nil
	}
	_, err = r.mutate(ctx, id, actor, "capability", func(n *Node) error {
		if n.Capability.Identity.AgeRecipient.Value != nil {
			if *n.Capability.Identity.AgeRecipient.Value != presented {
				return &Error{Code: "E660", Reason: "age_recipient_changed"}
			}
			return nil
		}
		n.Capability.Identity.AgeRecipient = model.Fact[string]{Value: &presented, Source: "self", ObservedAt: time.Now().UTC(), Confidence: "known"}
		return nil
	})
	var changed *Error
	if errors.As(err, &changed) && changed.Code == "E660" {
		_, flagErr := r.SetState(ctx, id, "maintenance", "age_recipient_changed", actor)
		if flagErr != nil {
			return flagErr
		}
	}
	return err
}
