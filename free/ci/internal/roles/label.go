package roles

// Purpose: ActorFromLabel and Current (D-1337): the dispatch of a queued run authorizes the submitter
// again, from the audit label the user API stored with the trigger, before the run starts.
// Inputs:  an Actor.Label string ("user:bob", "token:<id>:bob", "operator:cli:bob").
// Outputs: an Actor resolved from the current grant or token; an error when none holds a role.
// Constraints: the label is trusted only because the coordinator wrote it itself; the state behind it is
//          re-read here and again by every decision, so a revocation since the submit ends the access.
//          A certificate serial is not part of a label: a certificate revoked since the submit but
//          whose user still holds the grant is not caught here.

import (
	"context"
	"strings"
)

// Current is the Service that Use installed, or nil.
func Current() *Service { return current.Load() }

// ActorFromLabel resolves the actor a Label names.
func (s *Service) ActorFromLabel(ctx context.Context, label string) (Actor, error) {
	kind, rest, _ := strings.Cut(label, ":")
	switch Kind(kind) {
	case KindOperator:
		if rest != "" {
			return s.Operator(rest), nil
		}
	case KindUser:
		g, ok, err := s.St.GrantGet(ctx, rest)
		if err != nil {
			return Actor{}, err
		}
		if !ok {
			return Actor{}, fail(ReasonNoGrant, "user %s holds no role", rest)
		}
		return Actor{id: rest, kind: KindUser, role: Role(g.Role), projects: g.Projects, svc: s}, nil
	case KindToken:
		id, _, _ := strings.Cut(rest, ":")
		t, ok, err := s.St.TokenByID(ctx, id)
		if err != nil {
			return Actor{}, err
		}
		if err = s.tokenState(t, ok); err != nil {
			return Actor{}, err
		}
		return Actor{id: t.Principal, kind: KindToken, role: Role(t.Role), projects: t.Projects, tokenID: t.ID, svc: s}, nil
	}
	return Actor{}, fail(ReasonNoActor, "no identity")
}
