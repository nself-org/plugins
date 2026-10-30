package registry

import (
	"context"
	"errors"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// QuarantineReason is the maintenance reason set when a node presents a different age recipient (E660).
// SetState refuses to replace it (a drain, an enable or a breaker write would clear the quarantine).
const QuarantineReason = "age_recipient_changed"

// AgeRecipient returns the recipient pinned by the first hello, if any.
func (r *Registry) AgeRecipient(ctx context.Context, id string) (*string, error) {
	return r.Store.NodeAgeRecipient(ctx, id)
}

// CheckAgeRecipient pins once, then flags a changed recipient without replacing it.
func (r *Registry) CheckAgeRecipient(ctx context.Context, id, presented, actor string) error {
	if presented == "" {
		return invalid("age recipient required")
	}
	pin, err := r.Store.NodePinAgeRecipient(ctx, id, presented)
	if err != nil {
		return err
	}
	if pin != presented {
		if _, err = r.SetState(ctx, id, "maintenance", QuarantineReason, actor); err != nil {
			return err
		}
		return &Error{Code: "E660", Reason: QuarantineReason}
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
		_, flagErr := r.SetState(ctx, id, "maintenance", QuarantineReason, actor)
		if flagErr != nil {
			return flagErr
		}
	}
	return err
}
