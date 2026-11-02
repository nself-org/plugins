package roles

// Purpose: recover the authenticated identity stored with coordinator intake.
// Inputs: a server-recorded submitter label. Outputs: a live user or token actor.
// Constraints: labels never confer operator access; grants and tokens are reread.

import (
	"context"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/pki"
)

// ActorFromSubmitter resolves a durable intake label without restoring stale roles.
func (s *Service) ActorFromSubmitter(ctx context.Context, label string) (Actor, error) {
	parts := strings.Split(label, ":")
	if len(parts) == 2 && parts[0] == "user" && pki.ValidID(parts[1]) {
		a := Actor{id: parts[1], kind: KindUser, svc: s}
		return a.refresh(ctx)
	}
	if len(parts) == 3 && parts[0] == "token" && pki.ValidID(parts[1]) && pki.ValidID(parts[2]) {
		t, ok, err := s.St.TokenByID(ctx, parts[1])
		if err != nil {
			return Actor{}, err
		}
		if err := s.tokenState(t, ok); err != nil {
			return Actor{}, err
		}
		if t.Principal != parts[2] {
			return Actor{}, fail(ReasonNoActor, "submitter does not match token principal")
		}
		return Actor{id: t.Principal, kind: KindToken, tokenID: t.ID, role: Role(t.Role), projects: t.Projects, svc: s}, nil
	}
	return Actor{}, fail(ReasonNoActor, "no authenticated submitter")
}
