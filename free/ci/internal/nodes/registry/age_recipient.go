package registry

import (
	"context"
	"errors"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

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
